# Tenet Helm Chart

## How to use Tenet Helm repository

You need to add this repository to your Helm repositories:

```console
helm repo add tenet https://cybozu-go.github.io/tenet/
helm repo update
```

## Quick start

### Installing cert-manager

```console
$ curl -fsL https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml | kubectl apply -f -
```

### Installing the Chart

> NOTE:
>
> This installation method requires cert-manager to be installed beforehand.

To install the chart with the release name `tenet` using a dedicated namespace(recommended):

```console
$ helm install --create-namespace --namespace tenet tenet tenet/tenet
```

Specify parameters using `--set key=value[,key=value]` argument to `helm install`.

Alternatively a YAML file that specifies the values for the parameters can be provided like this:

```console
$ helm install --create-namespace --namespace tenet tenet -f values.yaml tenet/tenet
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controller.extraArgs | list | `[]` | Optional additional arguments. |
| controller.replicas | int | `2` | Specify the number of replicas of the controller Pod. |
| controller.resources | object | `{"requests":{"cpu":"100m","memory":"20Mi"}}` | Specify resources. |
| controller.terminationGracePeriodSeconds | int | `10` | Specify terminationGracePeriodSeconds. |
| image.pullPolicy | string | `nil` | Tenet image pullPolicy. |
| image.repository | string | `"ghcr.io/cybozu-go/tenet"` | Tenet image repository to use. |
| image.tag | string | `{{ .Chart.AppVersion }}` | Tenet image tag to use. |

## Generate Manifests

You can use the `helm template` command to render manifests.

```console
$ helm template --namespace tenet tenet tenet/tenet
```

## Upgrade CRDs

There is no support at this time for upgrading or deleting CRDs using Helm.
Users must manually upgrade the CRD if there is a change in the CRD used by Tenet.

https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#install-a-crd-declaration-before-using-the-resource

## Release Chart

Tenet Helm Chart will be released independently.
This will prevent the Tenet version from going up just by modifying the Helm Chart.

You must change the version of `Chart.yaml` when making changes to the Helm Chart.

Pushing a tag like `chart-v<chart version>` will cause GitHub Actions to release chart.
Chart versions are expected to follow [Semantic Versioning](https://semver.org/).
If the chart version in the tag does not match the version listed in `Chart.yaml`, the release will fail.
