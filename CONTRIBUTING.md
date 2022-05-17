# Contributing Guide

The Brigade CloudEvents Gateway is an official extension of the Brigade project
and as such follows all of the practices and policies laid out in the main
[Brigade Contributor Guide](https://docs.brigade.sh/topics/contributor-guide/).
Anyone interested in contributing to this gateway should familiarize themselves
with that guide _first_.

The remainder of _this_ document only supplements the above with things specific
to this project.

## Running `make hack-kind-up`

As with the main Brigade repository, running `make hack-kind-up` in this
repository will utilize [ctlptl](https://github.com/tilt-dev/ctlptl) and
[KinD](https://kind.sigs.k8s.io/) to launch a local, development-grade
Kubernetes cluster that is also connected to a local Docker registry.

In contrast to the main Brigade repo, this cluster is not pre-configured for
building and running Brigade itself from source, rather it is pre-configured for
building and running _this gateway_ from source. Because Brigade is a logical
prerequisite for this gateway to be useful in any way, `make hack-kind-up` will
pre-install a recent, _stable_ release of Brigade into the cluster.

## Running `tilt up`

As with the main Brigade repository, running `tilt up` will build and deploy
project code (the gateway, in this case) from source.

For the gateway to successfully communicate with the Brigade instance in your
local, development-grade cluster, you will need to execute the following steps
_before_ running `tilt up`:

1. Log into Brigade:

   ```shell
   $ brig login -k -s https://localhost:31600 --root
   ```

   The root password is `F00Bar!!!`.

1. Create a service account for the gateway:

   ```shell
   $ brig service-account create \
       --id cloudevents-gateway \
       --description cloudevents-gateway
   ```

1. Copy the token returned from the previous step and export it as the
   `BRIGADE_API_TOKEN` environment variable:

   ```shell
   $ export BRIGADE_API_TOKEN=<token from previous step>
   ```

1. Grant the service account permission to create events:

   ```shell
   $ brig role grant EVENT_CREATOR \
     --service-account cloudevents-gateway \
     --source brigade.sh/cloudevents
   ```

   > ⚠️&nbsp;&nbsp;Contributions that automate the creation and configuration of
   > the service account setup are welcome.

1. Edit the `tokens` section of `charts/brigade-cloudevents-gateway/values.yaml`
   to map cloudevent sources to tokens (shared secrets) that clients can use to
   authenticate to your gateway. See
   [this section](./README.md#2-install-the-cloudevents-gateway) of `README.md`
   for more information.

   > ⚠️&nbsp;&nbsp;Take care not to include modifications to the `values.yaml`
   > file in any PRs you open.

You can then run `tilt up` to build and deploy this gateway from source.

## Receiving Events Originating Locally

You can send cloudevents from a local client to `http://localhost:31700/events`.
The example below utilizes `curl`. Be sure to substitute an appropriate token
from the previous section in the `Authorization` header.

```shell
$ curl -i -k -X POST \
    -H "ce-specversion: 1.0" \
    -H "ce-id: 1234-1234-1234" \
    -H "ce-source: example/uri" \
    -H "ce-type: example.type" \
    -H "Authorization: Bearer MySharedSecret" \
    http://localhost:31700/events
```

## Receiving Events from External Services

Making the gateway that runs in your local, development-grade Kubernetes cluster
visible to services running elsewhere that may deliver cloudevents can be
challenging. To help ease this process, our `Tiltfile` has built-in support for
exposing your local gateway using [ngrok](https://ngrok.com/). To take advantage
of this:

1. [Sign up](https://dashboard.ngrok.com/signup) for a free ngrok account.

1. Follow ngrok
   [setup & installation instructions](https://dashboard.ngrok.com/get-started/setup)

1. Set the environment variable `ENABLE_NGROK_EXTENSION` to a value of `1`
   _before_ running `tilt up`.

1. After running `tilt up`, the option should become available in the Tilt UI at
  `http://localhost:10350/` to expose the gateway using ngrok. After going so,
   the applicable ngrok URL will be displayed in the gateway's logs in the Tilt
   UI.

1. Use the URL `<ngrok URL>/events` and token(s) (shared secrets) from the
   [Running `tilt up` section](#running-tilt-up) when configuring external
   services to deliver cloudevents to your gateway.

> ⚠️&nbsp;&nbsp;We cannot guarantee that ngrok will work in all environments,
> especially if you are behind a corporate firewall.
