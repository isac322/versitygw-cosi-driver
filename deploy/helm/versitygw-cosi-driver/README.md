# versitygw-cosi-driver

![Version](https://img.shields.io/badge/dynamic/yaml?url=https%3A%2F%2Fraw.githubusercontent.com%2Fisac322%2Fversitygw-cosi-driver%2Fmaster%2Fdeploy%2Fhelm%2Fversitygw-cosi-driver%2FChart.yaml&query=%24.version&label=Chart&color=blue)
![AppVersion](https://img.shields.io/badge/dynamic/yaml?url=https%3A%2F%2Fraw.githubusercontent.com%2Fisac322%2Fversitygw-cosi-driver%2Fmaster%2Fdeploy%2Fhelm%2Fversitygw-cosi-driver%2FChart.yaml&query=%24.appVersion&label=App&color=green)

Helm chart for the [VersityGW COSI Driver](https://github.com/isac322/versitygw-cosi-driver). Deploys a [COSI](https://github.com/kubernetes-sigs/container-object-storage-interface-spec) driver that manages S3 buckets and per-app credentials on [VersityGW](https://github.com/versity/versitygw) through Kubernetes custom resources (`BucketClaim`, `BucketAccess`).

See the [project README](https://github.com/isac322/versitygw-cosi-driver) for architecture, features, and troubleshooting.

## Prerequisites

- Kubernetes 1.25+ (required by COSI)
- Helm 3.x
- [COSI Controller](https://github.com/kubernetes-sigs/container-object-storage-interface) installed in the cluster
- A running [VersityGW](https://github.com/versity/versitygw) instance with IAM and Admin API enabled
- An admin credentials Secret in the target namespace

## Installing the Chart

```bash
# 1. Install the COSI controller (if not already installed)
kubectl create -k 'https://github.com/kubernetes-sigs/container-object-storage-interface//?ref=v0.2.2'

# 2. Create admin credentials Secret (if not already present)
kubectl create secret generic versitygw-root-credentials \
  --from-literal=rootAccessKeyId=YOUR_ACCESS_KEY \
  --from-literal=rootSecretAccessKey=YOUR_SECRET_KEY

# 3. Install the chart
helm install versitygw-cosi-driver \
  oci://ghcr.io/isac322/charts/versitygw-cosi-driver \
  --set driver.name=versitygw.cosi.dev
```

## Uninstalling the Chart

```bash
helm uninstall versitygw-cosi-driver
```

This does not delete BucketClass, BucketAccessClass, or any provisioned Bucket/BucketAccess resources. Clean those up separately if needed.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `driver.name` | string | `""` | COSI driver name (**required**). Must be unique per driver instance. |
| `driver.image.repository` | string | `ghcr.io/isac322/versitygw-cosi-driver` | Driver container image |
| `driver.image.tag` | string | `""` | Image tag (defaults to chart appVersion) |
| `driver.image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `driver.resources` | object | `{}` | Resource requests/limits for driver container |
| `driver.securityContext` | object | see `values.yaml` | Security context for driver container |
| `sidecar.image.repository` | string | `registry.k8s.io/sig-storage/objectstorage-sidecar` | COSI sidecar image |
| `sidecar.image.tag` | string | `v0.2.2` | Sidecar image tag |
| `sidecar.extraArgs` | list | `[]` | Extra arguments for sidecar (e.g. `["--v=5"]`) |
| `sidecar.resources` | object | `{}` | Resource requests/limits for sidecar container |
| `sidecar.securityContext` | object | see `values.yaml` | Security context for sidecar container |
| `versitygw.serviceName` | string | `versitygw` | VersityGW service name for endpoint derivation |
| `versitygw.s3Endpoint` | string | `""` | S3 API endpoint (derived from serviceName if empty) |
| `versitygw.s3Port` | int | `7070` | S3 API port (used when s3Endpoint is empty) |
| `versitygw.adminEndpoint` | string | `""` | Admin API endpoint (derived from serviceName if empty) |
| `versitygw.adminPort` | int | `7071` | Admin API port (used when adminEndpoint is empty) |
| `versitygw.region` | string | `us-east-1` | S3 region |
| `versitygw.credentials.secretName` | string | `versitygw-root-credentials` | Admin credentials Secret name |
| `versitygw.credentials.accessKeyField` | string | `rootAccessKeyId` | Access key field name in Secret |
| `versitygw.credentials.secretKeyField` | string | `rootSecretAccessKey` | Secret key field name in Secret |
| `serviceAccount.create` | bool | `true` | Create a ServiceAccount |
| `serviceAccount.name` | string | `""` | ServiceAccount name (generated if empty) |
| `serviceAccount.annotations` | object | `{}` | ServiceAccount annotations |
| `rbac.create` | bool | `true` | Create ClusterRole and ClusterRoleBinding |
| `bucketClass.create` | bool | `true` | Create a default BucketClass |
| `bucketClass.name` | string | `""` | BucketClass name (defaults to `versitygw`) |
| `bucketClass.deletionPolicy` | string | `Delete` | Bucket deletion policy |
| `bucketAccessClass.create` | bool | `true` | Create a default BucketAccessClass |
| `bucketAccessClass.name` | string | `""` | BucketAccessClass name (defaults to `versitygw`) |
| `bucketAccessClass.authenticationType` | string | `KEY` | Authentication type |
| `podSecurityContext` | object | see `values.yaml` | Pod-level security context |
| `nodeSelector` | object | `{}` | Node selector |
| `tolerations` | list | `[]` | Tolerations |
| `affinity` | object | `{}` | Affinity rules |
| `podAnnotations` | object | `{}` | Pod annotations |
| `podLabels` | object | `{}` | Pod labels |
| `imagePullSecrets` | list | `[]` | Image pull secrets |
