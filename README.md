# Brigade CloudEvents Gateway

[![codecov](https://codecov.io/gh/brigadecore/brigade-cloudevents-gateway/branch/main/graph/badge.svg?token=PM7LG36RGY)](https://codecov.io/gh/brigadecore/brigade-cloudevents-gateway)
[![Go Report Card](https://goreportcard.com/badge/github.com/brigadecore/brigade-cloudevents-gateway)](https://goreportcard.com/report/github.com/brigadecore/brigade-cloudevents-gateway)

This is a work-in-progress
[Brigade 2](https://github.com/brigadecore/brigade/tree/v2)
compatible gateway that receives [CloudEvents](https://cloudevents.io/) over 
HTTP/S and propagates them into Brigade 2's event bus.

## Installation

Prerequisites:

* A Kubernetes cluster:
    * For which you have the `admin` cluster role
    * That is already running Brigade 2
    * Capable of provisioning a _public IP address_ for a service of type
      `LoadBalancer`. (This means you won't have much luck running the gateway
      locally in the likes of kind or minikube unless you're able and willing to
      mess with port forwarding settings on your router, which we won't be
      covering here.)

* `kubectl`, `helm` (commands below assume Helm 3), and `brig` (the Brigade 2
  CLI)

### 1. Create a Service Account for the Gateway

__Note:__ To proceed beyond this point, you'll need to be logged into Brigade 2
as the "root" user (not recommended) or (preferably) as a user with the `ADMIN`
role. Further discussion of this is beyond the scope of this documentation.
Please refer to Brigade's own documentation.

Using Brigade 2's `brig` CLI, create a service account for the gateway to use:

```console
$ brig service-account create \
    --id brigade-cloudevents-gateway \
    --description brigade-cloudevents-gateway
```

Make note of the __token__ returned. This value will be used in another step.
_It is your only opportunity to access this value, as Brigade does not save it._

Authorize this service account to create new events:

```console
$ brig role grant EVENT_CREATOR \
    --service-account brigade-cloudevents-gateway \
    --source brigade.sh/cloudevents
```

__Note:__ The `--source brigade.sh/cloudevents` option specifies that this
service account can be used _only_ to create events having a value of
`brigade.sh/cloudevents` in the event's `source` field. _This is a security
measure that prevents the gateway from using this token for impersonating other
gateways._

### 2. Install the CloudEvents Gateway

For now, we're using the [GitHub Container Registry](https://ghcr.io) (which is
an [OCI registry](https://helm.sh/docs/topics/registries/)) to host our Helm
chart. Helm 3.7 has _experimental_ support for OCI registries. In the event that
the Helm 3.7 dependency proves troublesome for users, or in the event that this
experimental feature goes away, or isn't working like we'd hope, we will revisit
this choice before going GA.

First, be sure you are using
[Helm 3.7.0-rc.1](https://github.com/helm/helm/releases/tag/v3.7.0-rc.1) and
enable experimental OCI support:

```console
$ export HELM_EXPERIMENTAL_OCI=1
```

As this chart requires custom configuration as described above to function
properly, we'll need to create a chart values file with said config.

Use the following command to extract the full set of configuration options into
a file you can modify:

```console
$ helm inspect values oci://ghcr.io/brigadecore/brigade-cloudevents-gateway \
    --version v0.2.0 > ~/brigade-cloudevents-gateway-values.yaml
```

Edit `~/brigade-cloudevents-gateway-values.yaml`, making the following changes:

* `host`: Set this to the host name where you'd like the gateway to be
  accessible.

* `brigade.apiAddress`: Address of the Brigade API server, beginning with
  `https://`

* `brigade.apiToken`: Service account token from step 2

* `tokens`: This field should map CloudEvent sources to tokens (shared secrets)
  that can be used by clients to send cloud events for each source.
  
  Note that a Brigade event's source/type fields and a CloudEvent's source/type
  fields are similar in that they are both metadata that enable event routing,
  but will be different in value. The CloudEvent Gateway emits _Brigade_ events
  into Brigade's event bus with the source `brigade.sh/cloudevents` and type
  `cloudevent`. The CloudEvent's _original_ source and type become _qualifiers_
  on the Brigade event.

  For the example in sections 4 and 5 below, edit the token so that source
  `example/uri` authenticates using the token (shared secret) `MySharedSecret`.

Save your changes to `~/brigade-cloudevents-gateway-values.yaml` and use the following command to install
the gateway using the above customizations:

```console
$ helm install brigade-cloudevents-gateway \
    oci://ghcr.io/brigadecore/brigade-cloudevents-gateway \
    --version v0.2.0 \
    --create-namespace \
    --namespace brigade-cloudevents-gateway \
    --values ~/brigade-cloudevents-gateway-values.yaml
```

### 3. (RECOMMENDED) Create a DNS Entry

If you installed the gateway without enabling support for an ingress controller,
this command should help you find the gateway's public IP address:

```console
$ kubectl get svc brigade-cloudevents-gateway \
    --namespace brigade-cloudevents-gateway \
    --output jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

If you overrode defaults and enabled support for an ingress controller, you
probably know what you're doing well enough to track down the correct IP without
our help. 😉

With this public IP in hand, edit your name servers and add an `A` record
pointing your domain to the public IP.

### 4. Add a Brigade Project

You can create any number of Brigade projects (or modify an existing one) to
listen for CloudEvents that were sent to your gateway and, in turn, emitted into
Brigade's event bus:

```yaml
apiVersion: brigade.sh/v2-beta
kind: Project
metadata:
  id: cloudevents-demo
description: A project that demonstrates integration with CloudEvents
spec:
  eventSubscriptions:
  - source: brigade.sh/cloudevents
    types:
    - cloudevent
    qualifiers:
      source: example/uri
      type: example.type
  workerTemplate:
    defaultConfigFiles:
      brigade.js: |-
        const { events } = require("@brigadecore/brigadier");

        events.on("brigade.sh/cloudevents", "cloudevent", () => {
          console.log("Received an event from the brigade.sh/cloudevents gateway!");
        });

        events.process();
```

Assuming this file were named `project.yaml`, you can create the project like
so:

```console
$ brig project create --file project.yaml
```

### 5. Create a CloudEvent

You can use the following `curl` command to send a CloudEvent that should be
subscribed to by the example project in the previous section:

```console
$ curl -i -k -X POST \
    -H "ce-specversion: 1.0" \
    -H "ce-id: 1234-1234-1234" \
    -H "ce-source: example/uri" \
    -H "ce-type: example.type" \
    -H "Authorization: Bearer MySharedSecret" \
    https://<public IP or host name here>/events
```

If the gateway accepts the request, output will look like this:

```console
HTTP/1.1 200 OK
Date: Tue, 03 Aug 2021 19:13:37 GMT
Content-Length: 0
```

To confirm that the gateway emitted a corresponding Brigade event into Brigade's
event bus, list the events for the `cloudevents-demo` project (created in
section 4), which is subscribed to such events:

```console
$ brig event list --project cloudevents-demo
```

Full coverage of `brig` commands is beyond the scope of this documentation, but
at this point, additional `brig` commands can be applied to monitor the event's
status and view logs produced in the course of handling the event.

## Events Received and Emitted by this Gateway

CloudEvents received by this gateway are emitted into Brigade's event bus as
native Brigade events with source `brigade.sh/cloudevents` and type
`cloudevent`. The CloudEvent's original source and type are added to the native
Brigade event as qualifiers with the keys `source` and `type`, respectively. The
CloudEvent's original data (taken from a `data` field in the request body)
becomes the value of the native Brigade event's `payload` field.

## Examples Projects

See `examples/` for complete Brigade projects that demonstrate various
scenarios.

## Contributing

The Brigade project accepts contributions via GitHub pull requests. The
[Contributing](CONTRIBUTING.md) document outlines the process to help get your
contribution accepted.

## Support & Feedback

We have a slack channel!
[Kubernetes/#brigade](https://kubernetes.slack.com/messages/C87MF1RFD) Feel free
to join for any support questions or feedback, we are happy to help. To report
an issue or to request a feature open an issue
[here](https://github.com/brigadecore/brigade-cloudevents-gateway/issues)
