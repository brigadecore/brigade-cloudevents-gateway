import { events, Event, Job, ConcurrentGroup, SerialGroup, Container } from "@brigadecore/brigadier"

const goImg = "brigadecore/go-tools:v0.5.0"
const dindImg = "docker:20.10.9-dind"
const dockerClientImg = "brigadecore/docker-tools:v0.1.0"
const helmImg = "brigadecore/helm-tools:v0.4.0"
const localPath = "/workspaces/brigade-cloudevents-gateway"

// MakeTargetJob is just a job wrapper around one or more make targets.
class MakeTargetJob extends Job {
  constructor(targets: string[], img: string, event: Event, env?: {[key: string]: string}) {
    super(targets[0], img, event)
    this.primaryContainer.sourceMountPath = localPath
    this.primaryContainer.workingDirectory = localPath
    this.primaryContainer.environment = env || {}
    this.primaryContainer.environment["SKIP_DOCKER"] = "true"
    this.primaryContainer.command = [ "make" ]
    this.primaryContainer.arguments = targets
  }
}

// BuildImageJob is a specialized job type for building multiarch Docker images.
//
// Note: This isn't the optimal way to do this. It's a workaround. These notes
// are here so that as the situation improves, we can improve our approach.
//
// The optimal way of doing this would involve no sidecars and wouldn't closely
// resemble the "DinD" (Docker in Docker) pattern that we are accustomed to.
//
// `docker buildx build` has full support for building images using remote
// BuildKit instances. Such instances can use qemu to emulate other CPU
// architectures. This permits us to build images for arm64 (aka arm64/v8, aka
// aarch64), even though, as of this writing, we only have access to amd64 VMs.
//
// In an ideal world, we'd have a pool of BuildKit instances up and running at
// all times in our cluster and we'd somehow JOIN it and be off to the races.
// Alas, as of this writing, this isn't supported yet. (BuildKit supports it,
// but the `docker buildx` family of commands does not.) The best we can do is
// use `docker buildx create` to create a brand new builder.
//
// Tempting as it is to create a new builder using the Kubernetes driver (i.e.
// `docker buildx create --driver kubernetes`), this comes with two problems:
// 
// 1. It would require giving our jobs a lot of additional permissions that they
//    don't otherwise need (creating deployments, for instance). This represents
//    an attack vector I'd rather not open.
//
// 2. If the build should fail, nothing guarantees the builder gets shut down.
//    Over time, this could really clutter the cluster and starve us of
//    resources.
//
// The workaround I have chosen is to launch a new builder using the default
// docker-container driver. This runs inside a DinD sidecar. This has the
// benefit of always being cleaned up when the job is observed complete by the
// Brigade observer. The downside is that we're building an image inside a
// Russian nesting doll of containers with an ephemeral cache. It is slow, but
// it works.
//
// If and when the capability exists to use `docker buildx` with existing
// builders, we can streamline all of this pretty significantly.
class BuildImageJob extends MakeTargetJob {
  constructor(target: string, event: Event, env?: {[key: string]: string}) {
    super([target], dockerClientImg, event, env)
    this.primaryContainer.environment.DOCKER_HOST = "localhost:2375"
    this.primaryContainer.command = [ "sh" ]
    this.primaryContainer.arguments = [
      "-c",
      // The sleep is a grace period after which we assume the DinD sidecar is
      // probably up and running.
      `sleep 20 && docker buildx create --name builder --use && docker buildx ls && make ${target}`
    ]

    this.sidecarContainers.docker = new Container(dindImg)
    this.sidecarContainers.docker.privileged = true
    this.sidecarContainers.docker.environment.DOCKER_TLS_CERTDIR=""
  }
}

// PushImageJob is a specialized job type for publishing Docker images.
class PushImageJob extends BuildImageJob {
  constructor(target: string, event: Event, version?: string) {
    const env = {
      "DOCKER_ORG": event.project.secrets.dockerhubOrg,
      "DOCKER_USERNAME": event.project.secrets.dockerhubUsername,
      "DOCKER_PASSWORD": event.project.secrets.dockerhubPassword
    }
    if (version) {
      env["VERSION"] = version
    }
    super(target, event, env)
  }
}

// A map of all jobs. When a ci:job_requested event wants to re-run a single
// job, this allows us to easily find that job by name.
const jobs: {[key: string]: (event: Event) => Job } = {}

// Basic tests:

const testUnitJobName = "test-unit"
const testUnitJob = (event: Event) => {
  return new MakeTargetJob([testUnitJobName, "upload-code-coverage"], goImg, event, {
    "CODECOV_TOKEN": event.project.secrets.codecovToken
  })
}
jobs[testUnitJobName] = testUnitJob

const lintJobName = "lint"
const lintJob = (event: Event) => {
  return new MakeTargetJob([lintJobName], goImg, event)
}
jobs[lintJobName] = lintJob

const lintChartJobName = "lint-chart"
const lintChartJob = (event: Event) => {
  return new MakeTargetJob([lintChartJobName], helmImg, event)
}
jobs[lintChartJobName] = lintChartJob

// Build / publish stuff:

const buildJobName = "build"
const buildJob = (event: Event) => {
  return new BuildImageJob(buildJobName, event)
}
jobs[buildJobName] = buildJob

const pushJobName = "push"
const pushJob = (event: Event, version?: string) => {
  return new PushImageJob(pushJobName, event, version)
}
jobs[pushJobName] = pushJob

const publishChartJobName = "publish-chart"
const publishChartJob = (event: Event, version: string) => {
  return new MakeTargetJob([publishChartJobName], helmImg, event, {
    "VERSION": version,
    "HELM_REGISTRY": event.project.secrets.helmRegistry || "ghcr.io",
    "HELM_ORG": event.project.secrets.helmOrg,
    "HELM_USERNAME": event.project.secrets.helmUsername,
    "HELM_PASSWORD": event.project.secrets.helmPassword
  })
}

// Run the entire suite of tests WITHOUT publishing anything initially. If
// EVERYTHING passes AND this was a push (merge, presumably) to the main branch,
// then publish an "edge" image.
events.on("brigade.sh/github", "ci:pipeline_requested", async event => {
  await new SerialGroup(
    new ConcurrentGroup( // Basic tests
      testUnitJob(event),
      lintJob(event),
      lintChartJob(event)
    ),
    buildJob(event)
  ).run()
  if (event.worker?.git?.ref == "main") {
    // Push "edge" image
    await pushJob(event).run()
  }
})

// This event indicates a specific job is to be re-run.
events.on("brigade.sh/github", "ci:job_requested", async event => {
  const job = jobs[event.labels.job]
  if (job) {
    await job(event).run()
    return
  }
  throw new Error(`No job found with name: ${event.labels.job}`)
})

events.on("brigade.sh/github", "cd:pipeline_requested", async event => {
  const version = JSON.parse(event.payload).release.tag_name
  await new SerialGroup(
    pushJob(event, version),
    publishChartJob(event, version)
  ).run()
})

events.process()
