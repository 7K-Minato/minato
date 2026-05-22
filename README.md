# minato

minato is a Kubernetes-native platform for hosting persistent, multi-game dedicated game servers. This repository contains the operator API types, agent gRPC contract, and bootstrap scaffolding for the control plane and agents.

## Quickstart
```sh
make generate manifests
make install
kubectl apply -f config/samples/
```

## Getting Started

### Quickstart
```sh
make generate manifests
make install
kubectl apply -f config/samples/
```

### Prerequisites
- Go 1.22+
- Docker 17.03+
- kubectl 1.11.3+
- Access to a Kubernetes 1.11.3+ cluster
- kind
- buf
- protoc-gen-go, protoc-gen-go-grpc

### Setup
```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.8
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
```

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/minato:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Operator to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/minato:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from config/samples:

```sh
kubectl apply -f config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -f config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**Undeploy the operator from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/minato:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/minato/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
Run `make help` for a full list of targets.

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
