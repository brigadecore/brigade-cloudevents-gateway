import { events, Event, Job, ConcurrentGroup, SerialGroup, Container } from "@brigadecore/brigadier"

const goImg = "brigadecore/go-tools:v0.8.0"
const dindImg = "docker:20.10.9-dind"
const dockerClientImg = "brigadecore/docker-tools:v0.4.0"
const helmImg = "brigadecore/helm-tools:v0.4.0"
const localPath = "/workspaces/brigade-cloudevents-gateway"

// JobWithSource is a base class for any Job that uses project source code.
class JobWithSource extends Job {
  constructor(name: string, img: string, event: Event, env?: {[key: string]: string}) {
    super(name, img, event)
    this.primaryContainer.sourceMountPath = localPath
    this.primaryContainer.workingDirectory = localPath
    this.primaryContainer.environment = env || {}    
  }
}

// MakeTargetJob is just a job wrapper around one or more make targets.
class MakeTargetJob extends JobWithSource {
  constructor(name: string, targets: string[], img: string, event: Event, env?: {[key: string]: string}) {
    env ||= {}
    env["SKIP_DOCKER"] = "true"
    super(name, img, event, env)
    this.primaryContainer.command = [ "make" ]
    this.primaryContainer.arguments = targets
  }
}

// A map of all jobs. When a ci:job_requested event wants to re-run a single
// job, this allows us to easily find that job by name.
const jobs: {[key: string]: (event: Event, version?: string) => Job } = {}

// Basic tests:

const testUnitJobName = "test-unit"
const testUnitJob = (event: Event) => {
  return new MakeTargetJob(testUnitJobName, ["test-unit", "upload-code-coverage"], goImg, event, {
    "CODECOV_TOKEN": event.project.secrets.codecovToken
  })
}
jobs[testUnitJobName] = testUnitJob

const lintJobName = "lint"
const lintJob = (event: Event) => {
  return new MakeTargetJob(lintJobName, ["lint"], goImg, event)
}
jobs[lintJobName] = lintJob

const lintChartJobName = "lint-chart"
const lintChartJob = (event: Event) => {
  return new MakeTargetJob(lintChartJobName, ["lint-chart"], helmImg, event)
}
jobs[lintChartJobName] = lintChartJob

// Build / publish stuff:

// Build/push multiarch image.
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
const buildJobName = "build"
const buildJob = (event: Event, version?: string) => {
  const secrets = event.project.secrets
  const env = {
    // Use the Docker daemon that's running in a sidecar
    "DOCKER_HOST": "localhost:2375"
  }
  let registry: string
  let registryOrg: string
  let registryUsername: string
  let registryPassword: string
  let signingSetupCommands = ""
  let signingCommand = ""
  if (!version) { // This is where we'll push potentially unstable images
    registry = secrets.unstableImageRegistry
    registryOrg = secrets.unstableImageRegistryOrg
    registryUsername = secrets.unstableImageRegistryUsername
    registryPassword = secrets.unstableImageRegistryPassword
  } else { // This is where we'll push stable images only
    registry = secrets.stableImageRegistry
    registryOrg = secrets.stableImageRegistryOrg
    registryUsername = secrets.stableImageRegistryUsername
    registryPassword = secrets.stableImageRegistryPassword
    // Since it's defined, the make target will want this env var
    env["VERSION"] = version
    env["BASE64_IMAGE_SIGNING_KEY"] = secrets.base64ImageSigningKey
    // This env var is documented here:
    // https://docs.docker.com/engine/security/trust/trust_automation/
    env["DOCKER_CONTENT_TRUST_REPOSITORY_PASSPHRASE"] = secrets.imageSigningKeyPassphrase
    const keyDir = "~/.docker/trust/private"
    const keyFile = `${keyDir}/${secrets.imageSigningKeyHash}.key`
    signingSetupCommands = `mkdir -p ${keyDir} && chmod 700 ${keyDir} && ` +
      `printf $BASE64_IMAGE_SIGNING_KEY | base64 -d > ${keyFile} && chmod 600 ${keyFile} && ` +
      `docker trust key load --name ${registryUsername} ${keyFile} && `
    signingCommand = " && make sign"
  }
  if (registry) {
    // Since it's defined, the make target will want this env var
    env["DOCKER_REGISTRY"] = registry
  }
  if (registryOrg) {
    // Since it's defined, the make target will want this env var
    env["DOCKER_ORG"] = registryOrg
  }
  // We ALWAYS log in to Docker Hub because even if we plan to push the images
  // elsewhere, we still PULL a lot of images from Docker Hub (in FROM
  // directives of Dockerfiles) and we want to avoid being rate limited.
  env["DOCKERHUB_PASSWORD"] = secrets.dockerhubPassword
  let registriesLoginCmd = `docker login -u ${secrets.dockerhubUsername} -p $DOCKERHUB_PASSWORD`
  // If the registry we push to is defined (not DockerHub; which we're already
  // logging into) and we have credentials, add a second registry login:
  if (registry && registryUsername && registryPassword) {
    env["IMAGE_REGISTRY_PASSWORD"] = registryPassword
    registriesLoginCmd = `${registriesLoginCmd} && docker login ${registry} -u ${registryUsername} -p $IMAGE_REGISTRY_PASSWORD`
  }
  const job = new JobWithSource(buildJobName, dockerClientImg, event, env)
  job.primaryContainer.command = [ "sh" ]
  job.primaryContainer.arguments = [
    "-c",
    // The sleep is a grace period after which we assume the DinD sidecar is
    // probably up and running.
    "sleep 20 && " +
      `${registriesLoginCmd} && ` +
      signingSetupCommands +
      "docker buildx create --name builder --use && " +
      "docker info && " +
      "make push" +
      signingCommand
  ]
  job.sidecarContainers.dind = new Container(dindImg)
  job.sidecarContainers.dind.privileged = true
  job.sidecarContainers.dind.environment["DOCKER_TLS_CERTDIR"] = ""
  return job
}
jobs[buildJobName] = buildJob

const scanJobName = "scan"
const scanJob = (event: Event) => {
  const env = {}
  const secrets = event.project.secrets
  if (secrets.unstableImageRegistry) {
    env["DOCKER_REGISTRY"] = secrets.unstableImageRegistry
  }
  if (secrets.unstableImageRegistryOrg) {
    env["DOCKER_ORG"] = secrets.unstableImageRegistryOrg
  }
  const job = new MakeTargetJob(scanJobName, ["scan"], dockerClientImg, event, env)
  job.fallible = true
  return job
}
jobs[scanJobName] = scanJob

const publishSBOMJobName = "publish-sbom"
const publishSBOMJob = (event: Event, version: string) => {
  const secrets = event.project.secrets
  const env = {
    "GITHUB_ORG": secrets.githubOrg,
    "GITHUB_REPO": secrets.githubRepo,
    "GITHUB_TOKEN": secrets.githubToken,
    "VERSION": version
  }
  if (secrets.stableImageRegistry) {
    env["DOCKER_REGISTRY"] = secrets.stableImageRegistry
  }
  if (secrets.stableImageRegistryOrg) {
    env["DOCKER_ORG"] = secrets.stableImageRegistryOrg
  }
  return new MakeTargetJob(publishSBOMJobName, ["publish-sbom"], dockerClientImg, event, env)
}
jobs[publishSBOMJobName] = publishSBOMJob

const publishChartJobName = "publish-chart"
const publishChartJob = (event: Event, version: string) => {
  return new MakeTargetJob(publishChartJobName, ["publish-chart"], helmImg, event, {
    "VERSION": version,
    "HELM_REGISTRY": event.project.secrets.helmRegistry || "ghcr.io",
    "HELM_ORG": event.project.secrets.helmOrg,
    "HELM_USERNAME": event.project.secrets.helmUsername,
    "HELM_PASSWORD": event.project.secrets.helmPassword
  })
}

events.on("brigade.sh/github", "ci:pipeline_requested", async event => {
  await new SerialGroup(
    new ConcurrentGroup( // Basic tests
      testUnitJob(event),
      lintJob(event),
      lintChartJob(event)
    ),
    buildJob(event),
    scanJob(event)
  ).run()
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
    buildJob(event, version),
    publishChartJob(event, version),
    publishSBOMJob(event, version)
  ).run()
})

events.process()
