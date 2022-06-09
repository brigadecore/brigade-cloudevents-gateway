# Installation

## Prerequisites

* A Kubernetes cluster:
    * For which you have the `admin` cluster role.
    * That is already running Brigade v2.6.0 or greater.
    * That is capable of provisioning a _public IP address_ for a service of type
    `LoadBalancer`.

    > ⚠️&nbsp;&nbsp;This means you won't have much luck running the gateway
    > locally in the likes of [KinD](https://kind.sigs.k8s.io/) or
    > [minikube](https://minikube.sigs.k8s.io/docs/) unless you're able and
    > willing to create port forwarding rules on your router or make use of a
    > service such as [ngrok](https://ngrok.com/). Both of these are beyond
    > the scope of this documentation.
* `kubectl`
* `helm`: Commands below require `helm` 3.7.0+.
* `brig`: The Brigade CLI. Commands below require `brig` 2.6.0+.

## Create a Service Account for the Gateway

> ⚠️&nbsp;&nbsp;To proceed beyond this point, you'll need to be logged into
> Brigade as the "root" user (not recommended) or (preferably) as a user with
> the `ADMIN` role. Further discussion of this is beyond the scope of this
> documentation. Please refer to Brigade's own documentation.

1. Using the `brig` CLI, create a service account for the gateway to use:

   ```shell
   $ brig service-account create \
       --id brigade-cloudevents-gateway \
       --description "Used by the Brigade CloudEvents Gateway"
   ```

1. Make note of the __token__ returned. This value will be used in another step.

   > ⚠️&nbsp;&nbsp;This is your only opportunity to access this value, as
   > Brigade does not save it.

1. Authorize this service account to create events:

   ```shell
   $ brig role grant EVENT_CREATOR \
       --service-account brigade-cloudevents-gateway \
       --source brigade.sh/cloudevents
   ```

   > ⚠️&nbsp;&nbsp;The `--source brigade.sh/cloudevents` option specifies that
   > this service account can be used _only_ to create events having a value of
   > `brigade.sh/cloudevents` in the event's `source` field. This is a security
   > measure that prevents the gateway from using this token for impersonating
   > other gateways.

## Install the Gateway

> ⚠️&nbsp;&nbsp;Be sure you are using
> [Helm 3.7.0](https://github.com/helm/helm/releases/tag/v3.7.0) or greater and
> enable experimental OCI support:
>
> ```shell
> $ export HELM_EXPERIMENTAL_OCI=1
> ```

1. As this gateway requires some specific configuration to function properly,
   we'll first create a values file containing those settings. Use the following
   command to extract the full set of configuration options into a file you can
   modify:

   ```shell
   $ helm inspect values oci://ghcr.io/brigadecore/brigade-cloudevents-gateway \
    --version v1.0.0 > ~/brigade-cloudevents-gateway-values.yaml
   ```

1. Edit `~/brigade-cloudevents-gateway-values.yaml`, making the following
   changes:

   * `host`: Set this to the host name where you'd like the gateway to be
     accessible.

   * `brigade.apiAddress`: Set this to the address of the Brigade API server,
     beginning with `https://`.

   * `brigade.apiToken`: Set this to the service account token obtained when you
     created the Brigade service account for this gateway.

   * `service.type`: If you plan to enable ingress (advanced), you can leave
     this as its default -- `ClusterIP`. If you do not plan to enable ingress,
     you probably will want to change this value to `LoadBalancer`.

    * `tokens`: This field should define tokens that can be used by clients to
      send events (webhooks) to this gateway. Note that keys are completely
      ignored by the gateway and only the values (tokens) matter. The keys only
      serve as recognizable token identifiers for human operators.

   > ⚠️&nbsp;&nbsp;By default, TLS will be enabled and a self-signed certificate
   > will be generated.
   >
   > For a production-grade deployment you should explore the options available
   > for providing or provisioning a certificate signed by a trusted authority.
   > These options can be located under the `tls` and `ingress.tls` sections of
   > the values file.

1. Save your changes to `~/brigade-cloudevents-gateway-values.yaml`.

1. Use the following command to install the gateway:

```shell
$ helm install brigade-cloudevents-gateway \
    oci://ghcr.io/brigadecore/brigade-cloudevents-gateway \
    --version v1.0.0 \
    --create-namespace \
    --namespace brigade-cloudevents-gateway \
    --values ~/brigade-cloudevents-gateway-values.yaml \
    --wait \
    --timeout 300s
```

## (RECOMMENDED) Create a DNS Entry

If you overrode defaults and set `service.type` to `LoadBalancer`, use this
command to find the gateway's public IP address:

```shell
$ kubectl get svc brigade-cloudevents-gateway \
    --namespace brigade-cloudevents-gateway \
    --output jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

If you overrode default configuration to enable support for an ingress
controller, you probably know what you're doing well enough to track down the
correct IP for that ingress controller without our help. 😉

With this public IP in hand, optionally edit your name servers and add an `A`
record pointing a domain name to the public IP.
