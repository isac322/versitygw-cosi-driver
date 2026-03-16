# VersityGW COSI Driver

[![Go Reference](https://pkg.go.dev/badge/github.com/isac322/versitygw-cosi-driver.svg)](https://pkg.go.dev/github.com/isac322/versitygw-cosi-driver)
[![License](https://img.shields.io/github/license/isac322/versitygw-cosi-driver)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/isac322/versitygw-cosi-driver)](https://goreportcard.com/report/github.com/isac322/versitygw-cosi-driver)
[![Release](https://img.shields.io/github/v/release/isac322/versitygw-cosi-driver)](https://github.com/isac322/versitygw-cosi-driver/releases)

A Kubernetes [COSI](https://container-object-storage-interface.github.io/) driver that brings native S3 bucket provisioning and access management to [VersityGW](https://github.com/versity/versitygw) — manage object storage buckets and credentials as Kubernetes resources.

## Overview

**COSI (Container Object Storage Interface)** is the Kubernetes-native standard for managing object storage, just as CSI is for block/file storage. This driver implements the COSI specification for VersityGW, enabling you to:

- Declaratively create and delete S3 buckets via `BucketClaim`
- Automatically provision scoped credentials via `BucketAccess`
- Manage all object storage lifecycle through standard Kubernetes workflows

No more manual bucket creation or credential distribution — just apply YAML manifests and let Kubernetes handle the rest.

## Features

- **Dynamic Bucket Provisioning** — Create and delete S3 buckets through Kubernetes `BucketClaim` resources
- **Automated Access Management** — Provision per-application IAM users with scoped bucket policies via `BucketAccess`
- **Kubernetes-Native** — Fully implements the COSI gRPC `Provisioner` and `Identity` services
- **Secure by Default** — Runs as non-root in a distroless container with read-only filesystem
- **Helm Chart Included** — Production-ready deployment with RBAC, COSI sidecar, and configurable values

## Prerequisites

- Kubernetes 1.25+
- [COSI Controller](https://github.com/kubernetes-sigs/container-object-storage-interface) installed in the cluster
- A running [VersityGW](https://github.com/versity/versitygw) instance with S3 and Admin API endpoints

## Quick Start

### 1. Install the COSI Controller

Follow the [COSI installation guide](https://container-object-storage-interface.github.io/docs/deployment/installation) to install the controller in your cluster.

### 2. Create Admin Credentials Secret

```bash
kubectl create secret generic versitygw-admin-credentials \
  --from-literal=accessKey=<ADMIN_ACCESS_KEY> \
  --from-literal=secretKey=<ADMIN_SECRET_KEY>
```

### 3. Install the Driver

```bash
helm install versitygw-cosi-driver ./deploy/helm/versitygw-cosi-driver/ \
  --set driver.name=versitygw.cosi.dev \
  --set versitygw.s3Endpoint=http://versitygw:7070 \
  --set versitygw.adminEndpoint=http://versitygw:7071 \
  --set versitygw.credentials.secretName=versitygw-admin-credentials
```

### 4. Provision a Bucket

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: my-bucket
  namespace: default
spec:
  bucketClassName: versitygw
  protocols:
    - s3
```

## Usage

### Request Bucket Credentials

Create a `BucketAccess` to get scoped S3 credentials for your application:

```yaml
apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketAccess
metadata:
  name: my-bucket-access
  namespace: default
spec:
  bucketClaimName: my-bucket
  bucketAccessClassName: versitygw
  credentialsSecretName: my-bucket-credentials
  protocol: s3
```

The driver creates a dedicated IAM user, attaches a bucket policy, and stores S3 credentials (`accessKeyID`, `accessSecretKey`, `endpoint`, `region`) in the specified Kubernetes Secret.

## Configuration

### Helm Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `driver.name` | COSI driver name (required) | `""` |
| `versitygw.s3Endpoint` | VersityGW S3 API endpoint | `http://versitygw:7070` |
| `versitygw.adminEndpoint` | VersityGW Admin API endpoint | `http://versitygw:7071` |
| `versitygw.region` | S3 region | `us-east-1` |
| `versitygw.credentials.secretName` | Name of the admin credentials Secret | `versitygw-admin-credentials` |
| `driver.image.repository` | Driver container image | `versitygw-cosi-driver` |
| `driver.image.tag` | Image tag (defaults to chart appVersion) | `""` |
| `bucketClass.create` | Create a default BucketClass | `true` |
| `bucketAccessClass.create` | Create a default BucketAccessClass | `true` |

See [`deploy/helm/versitygw-cosi-driver/values.yaml`](deploy/helm/versitygw-cosi-driver/values.yaml) for all available options.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `VERSITYGW_S3_ENDPOINT` | VersityGW S3 API endpoint URL |
| `VERSITYGW_ADMIN_ENDPOINT` | VersityGW Admin API endpoint URL |
| `VERSITYGW_ADMIN_ACCESS` | Admin access key |
| `VERSITYGW_ADMIN_SECRET` | Admin secret key |
| `DRIVER_NAME` | COSI driver name (required) |
| `VERSITYGW_REGION` | S3 region (default: `us-east-1`) |

## Architecture

```
┌─────────────────────────────────────────────┐
│  Kubernetes Cluster                         │
│                                             │
│  BucketClaim ──► COSI Controller            │
│  BucketAccess ──► COSI Sidecar ──► Driver ──┼──► VersityGW
│                        (gRPC)               │    (S3 + Admin API)
└─────────────────────────────────────────────┘
```

The driver runs as a Kubernetes Deployment with two containers:
1. **versitygw-cosi-driver** — implements the COSI gRPC `Provisioner` and `Identity` services
2. **objectstorage-sidecar** — the standard COSI sidecar that bridges Kubernetes resources to gRPC calls

They communicate over a shared Unix socket.

## Contributing

```bash
# Build
make build

# Unit tests
make test

# Integration tests (requires versitygw binary in PATH)
make integration-test
```

## License

See [LICENSE](LICENSE) for details.
