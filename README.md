# maas-operator

A Kubernetes operator for managing Model-as-a-Service (MaaS) platform deployments on Kubernetes and OpenShift clusters.

The maas-operator automates the installation and configuration of the MaaS platform infrastructure, including gateway setup, authentication policies, rate limiting, and observability components.

## Getting Started

### Prerequisites

- **go** version v1.24.0+
- **kubectl** version v1.11.3+
- **kind** for local development (see [kind installation guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation))
- **podman** or **docker** version 17.03+ (for building container images)
- Access to a Kubernetes v1.11.3+ cluster (for production deployments)

### Quick Start

The fastest way to get started is using the provided quickstart script. It supports three deployment modes:
1. **Kind cluster** (local development)
2. **Direct deployment** to existing Kubernetes/OpenShift clusters
3. **OLM deployment** to OpenShift (shows up in "Installed Operators" console)

#### Option 1: Deploy to Kind Cluster (Local Development)

Deploy to a new Kind cluster (default mode):

```bash
./scripts/quickstart.sh
```

Or explicitly specify Kind mode:

```bash
./scripts/quickstart.sh --kind
```

This will:
1. Create a Kind cluster named `maas-operator` (or use `KIND_CLUSTER` env var to customize)
2. Build the operator container image using Podman
3. Load the image into the Kind cluster
4. Install the CRDs
5. Deploy the operator

**Note:** The script defaults to using `podman` as the container tool. To use Docker instead:

```bash
# Using Podman (recommended)
./scripts/quickstart.sh

# Docker alternative:
CONTAINER_TOOL=docker ./scripts/quickstart.sh
```

To clean up the Kind cluster:

```bash
kind delete cluster --name maas-operator
```

#### Option 2: Deploy to Existing Kubernetes Cluster (Direct)
```````````
Deploy to an existing cluster with image push to registry:

```bash
# Using Podman (recommended)
./scripts/quickstart.sh --cluster --image quay.io/maas/maas-operator:v0.1.0 --push

# Docker alternative:
CONTAINER_TOOL=docker ./scripts/quickstart.sh --cluster --image quay.io/maas/maas-operator:v0.1.0 --push
```

Deploy to existing cluster without building (image already exists in registry):

```bash
./scripts/quickstart.sh --cluster --image quay.io/maas/maas-operator:v0.1.0 --no-build
```

**Note:** This deployment method will NOT show up in the OpenShift console's "Installed Operators" page. Use `--olm` mode for that (see below).

#### Option 3: Deploy to OpenShift using OLM (Recommended for OpenShift)

Deploy using Operator Lifecycle Manager (OLM) - this will show up in the OpenShift console under "Installed Operators":

```bash
# Using Podman (recommended) - build and push all images
./scripts/quickstart.sh --olm --image quay.io/maas/maas-operator:v0.1.0 --version 0.1.0 --push

# Docker alternative:
CONTAINER_TOOL=docker ./scripts/quickstart.sh --olm --image quay.io/maas/maas-operator:v0.1.0 --version 0.1.0 --push

# If images already exist in registry
./scripts/quickstart.sh --olm --image quay.io/maas/maas-operator:v0.1.0 --version 0.1.0 --no-build
```

This will:
1. Generate the OLM bundle manifests
2. Build and push the operator, bundle, and catalog images
3. Create a CatalogSource in `openshift-marketplace`
4. Make the operator available in OperatorHub

After the CatalogSource is created, install via:
- **OpenShift Console**: Navigate to Operators → OperatorHub → Search for "MaaS Operator"
- **CLI**: Follow the instructions printed by the script

**Prerequisites for OLM deployment:**
- `operator-sdk` CLI installed
- Your `kubectl` is configured to connect to the OpenShift cluster
- The image registry is accessible from your cluster
- You have proper RBAC permissions to create CatalogSources

**General Prerequisites:**
When deploying to existing clusters, ensure:
- Your `kubectl` is configured to connect to the target cluster
- The image registry is accessible from your cluster
- You have proper RBAC permissions to install CRDs and create deployments

After deployment, create a MaasPlatform resource:

```bash
kubectl apply -k config/samples/
```

For more options, see the script help:

```bash
./scripts/quickstart.sh --help
```

### Deploy to the Cluster

For production deployments or existing clusters:

**Build and push your image to the location specified by `IMG`:**

```bash
# Using Podman (recommended)
make docker-build docker-push IMG=quay.io/maas/maas-operator:tag CONTAINER_TOOL=podman

# Docker alternative:
make docker-build docker-push IMG=quay.io/maas/maas-operator:tag CONTAINER_TOOL=docker
```

**NOTE:** This image should be published in a registry that your cluster can access.
Make sure you have the proper permissions to push to the registry.

**Install the CRDs into the cluster:**

```bash
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```bash
make deploy IMG=quay.io/maas/maas-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
> privileges or be logged in as admin.

**Create instances of your solution:**

You can apply the samples (examples) from the config/samples using kustomize:

```bash
kubectl apply -k config/samples/
```

> **NOTE**: Ensure that the samples have default values to test it out.

### To Uninstall

**Delete the instances (CRs) from the cluster:**

```bash
kubectl delete -k config/samples/
```

**Delete the APIs (CRDs) from the cluster:**

```bash
make uninstall
```

**Undeploy the controller from the cluster:**

```bash
make undeploy
```

**NOTE:** Run `make help` for more information on all potential `make` targets.

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Documentation

- **[Deployment Guide](docs/deployment.md)**: Detailed guide for deploying MaasPlatform and Tier resources
- **[Setup Test Dependencies](scripts/setup-test-dependencies.sh)**: Script to install ODH and Kuadrant operators for testing

## Customizing Operator Metadata

The operator's display information for OLM (Operator Lifecycle Manager) is defined in:

```
config/manifests/bases/maas-operator.clusterserviceversion.yaml
```

You can customize:
- **Display Name**: How the operator appears in OperatorHub
- **Description**: Detailed description and feature list
- **Icon**: Base64-encoded SVG icon
- **Keywords**: Search terms for OperatorHub
- **Maintainers**: Contact information
- **Links**: Documentation and related project links
- **Categories**: OperatorHub categories (e.g., "AI/Machine Learning")

After modifying this file, regenerate the bundle with:

```bash
make bundle VERSION=x.y.z IMG=your-image:tag
```

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

