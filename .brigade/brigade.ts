import { events, Event, Job, ConcurrentGroup, SerialGroup } from "@brigadecore/brigadier"

const releaseTagRegex = /^refs\/tags\/(v[0-9]+(?:\.[0-9]+)*(?:\-.+)?)$/

const goImg = "brigadecore/go-tools:v0.1.0"
const kanikoImg = "brigadecore/kaniko:v0.2.0"
const helmImg = "brigadecore/helm-tools:v0.1.0"
const localPath = "/workspaces/brigade-cloudevents-gateway"

// MakeTargetJob is just a job wrapper around a make target.
class MakeTargetJob extends Job {
  constructor(target: string, img: string, event: Event, env?: {[key: string]: string}) {
    super(target, img, event)
    this.primaryContainer.sourceMountPath = localPath
    this.primaryContainer.workingDirectory = localPath
    this.primaryContainer.environment = env || {}
    this.primaryContainer.environment["SKIP_DOCKER"] = "true"
    if (event.worker?.git?.ref) {
      const matchStr = event.worker.git.ref.match(releaseTagRegex)
      if (matchStr) {
        this.primaryContainer.environment["VERSION"] = Array.from(matchStr)[1] as string
      }
    }
    this.primaryContainer.command = [ "make" ]
    this.primaryContainer.arguments = [ target ]
  }
}

// PushImageJob is a specialized job type for publishing Docker images.
class PushImageJob extends MakeTargetJob {
  constructor(target: string, event: Event) {
    super(target, kanikoImg, event, {
      "DOCKER_ORG": event.project.secrets.dockerhubOrg,
      "DOCKER_USERNAME": event.project.secrets.dockerhubUsername,
      "DOCKER_PASSWORD": event.project.secrets.dockerhubPassword
    })
  }
}

// A map of all jobs. When a check_run:rerequested event wants to re-run a
// single job, this allows us to easily find that job by name.
const jobs: {[key: string]: (event: Event) => Job } = {}

// Basic tests:

const testUnitJobName = "test-unit"
const testUnitJob = (event: Event) => {
  return new MakeTargetJob(testUnitJobName, goImg, event)
}
jobs[testUnitJobName] = testUnitJob

const lintJobName = "lint"
const lintJob = (event: Event) => {
  return new MakeTargetJob(lintJobName, goImg, event)
}
jobs[lintJobName] = lintJob

const lintChartJobName = "lint-chart"
const lintChartJob = (event: Event) => {
  return new MakeTargetJob(lintChartJobName, helmImg, event)
}
jobs[lintChartJobName] = lintChartJob

// Build / publish stuff:

const buildJobName = "build"
const buildJob = (event: Event) => {
  return new MakeTargetJob(buildJobName, kanikoImg, event)
}
jobs[buildJobName] = buildJob

const pushJobName = "push"
const pushJob = (event: Event) => {
  return new PushImageJob(pushJobName, event)
}
jobs[pushJobName] = pushJob

const publishChartJobName = "publish-chart"
const publishChartJob = (event: Event) => {
  return new MakeTargetJob(publishChartJobName, helmImg, event, {
    "HELM_REGISTRY": event.project.secrets.helmRegistry || "ghcr.io",
    "HELM_ORG": event.project.secrets.helmOrg,
    "HELM_USERNAME": event.project.secrets.helmUsername,
    "HELM_PASSWORD": event.project.secrets.helmPassword
  })
}
jobs[publishChartJobName] = publishChartJob

// Run the entire suite of tests WITHOUT publishing anything initially. If
// EVERYTHING passes AND this was a push (merge, presumably) to the master
// branch, then publish an "edge" image.
async function runSuite(event: Event): Promise<void> {
  await new SerialGroup(
    new ConcurrentGroup( // Basic tests
      testUnitJob(event),
      lintJob(event),
      lintChartJob(event)
    ),
    buildJob(event)
  ).run()
  if (event.worker?.git?.ref == "master") {
    // Push "edge" images.
    //
    // npm packages MUST be semantically versioned, so we DON'T publish an
    // edge brigadier package.
    //
    // To keep our github released page tidy, we're also not publishing "edge"
    // CLI binaries.
    await pushJob(event).run()
  }
}

// Either of these events should initiate execution of the entire test suite.
events.on("brigade.sh/github", "check_suite:requested", runSuite)
events.on("brigade.sh/github", "check_suite:rerequested", runSuite)

// This event indicates a specific job is to be re-run.
events.on("brigade.sh/github", "check_run:rerequested", async event => {
  // Check run names are of the form <project name>:<job name>, so we strip
  // event.project.id.length + 1 characters off the start of the check run name
  // to find the job name.
  const jobName = JSON.parse(event.payload).check_run.name.slice(event.project.id.length + 1)
  const job = jobs[jobName]
  if (job) {
    await job(event).run()
    return
  }
  throw new Error(`No job found with name: ${jobName}`)
})

// Pushing new commits to any branch in github triggers a check suite. Such
// events are already handled above. Here we're only concerned with the case
// wherein a new TAG has been pushed-- and even then, we're only concerned with
// tags that look like a semantic version and indicate a formal release should
// be performed.
events.on("brigade.sh/github", "push", async event => {
  const matchStr = event.worker.git.ref.match(releaseTagRegex)
  if (matchStr) {
    // This is an official release with a semantically versioned tag
    await new SerialGroup(
      pushJob(event),
      publishChartJob(event)
    ).run()
  } else {
    console.log(`Ref ${event.worker.git.ref} does not match release tag regex (${releaseTagRegex}); not releasing.`)
  }
})

events.process()
