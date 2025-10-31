#!/usr/bin/env bash

# Quickstart script for maas-operator
# This script can either create a Kind cluster or deploy to an existing Kubernetes/OpenShift cluster

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default values
CLUSTER_NAME="${KIND_CLUSTER:-maas-operator}"
IMAGE_NAME="${IMG:-controller:latest}"
CONTAINER_TOOL="${CONTAINER_TOOL:-podman}"
DEPLOY_MODE="kind"  # "kind" or "cluster"
BUILD_IMAGE=true
PUSH_IMAGE=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
info() {
    echo -e "${GREEN}ℹ${NC} $1"
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}❌${NC} $1"
}

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Deploy maas-operator to a Kind cluster or an existing Kubernetes/OpenShift cluster.

Options:
    --kind                    Deploy to a Kind cluster (default)
    --cluster                 Deploy to an existing Kubernetes/OpenShift cluster
    --image IMAGE             Container image name (default: controller:latest)
    --no-build                Skip building the image (use existing image)
    --push                    Push image to registry (required for existing clusters)
    --container-tool TOOL      Container tool to use: podman or docker (default: podman)
    -h, --help               Show this help message

Environment Variables:
    KIND_CLUSTER              Kind cluster name (default: maas-operator)
    IMG                       Container image name (default: controller:latest)
    CONTAINER_TOOL            Container tool: podman or docker (default: podman)

Examples:
    # Deploy to a new Kind cluster (default)
    $0

    # Deploy to existing Kubernetes cluster with image push
    $0 --cluster --image quay.io/maas/maas-operator:v0.1.0 --push

    # Deploy to existing cluster without building (image already exists)
    $0 --cluster --image quay.io/maas/maas-operator:v0.1.0 --no-build

EOF
}

# Check if command exists
check_command() {
    if ! command -v "$1" &> /dev/null; then
        error "$1 is not installed. Please install it first."
        exit 1
    fi
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --kind)
            DEPLOY_MODE="kind"
            shift
            ;;
        --cluster)
            DEPLOY_MODE="cluster"
            shift
            ;;
        --image)
            IMAGE_NAME="$2"
            shift 2
            ;;
        --no-build)
            BUILD_IMAGE=false
            shift
            ;;
        --push)
            PUSH_IMAGE=true
            shift
            ;;
        --container-tool)
            CONTAINER_TOOL="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Validate deployment mode
if [[ "$DEPLOY_MODE" == "cluster" ]] && [[ "$BUILD_IMAGE" == true ]] && [[ "$PUSH_IMAGE" == false ]]; then
    warn "Deploying to existing cluster with local image build."
    warn "The image will only be available locally. Use --push to push to a registry,"
    warn "or use --no-build if the image already exists in your registry."
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        info "Aborted. Use --push to push the image to a registry."
        exit 0
    fi
fi

# Check prerequisites
info "Checking prerequisites..."
check_command kubectl
check_command make

if [[ "$DEPLOY_MODE" == "kind" ]]; then
    check_command kind
fi

# Check container tool
if [[ "$BUILD_IMAGE" == true ]]; then
    if ! command -v "$CONTAINER_TOOL" &> /dev/null; then
        error "$CONTAINER_TOOL is not installed. Please install it first."
        exit 1
    fi
    info "Using container tool: $CONTAINER_TOOL"
fi

# Verify cluster connectivity
info "Verifying cluster connectivity..."
if ! kubectl cluster-info &> /dev/null; then
    error "Cannot connect to Kubernetes cluster. Please check your kubeconfig."
    exit 1
fi

CURRENT_CONTEXT=$(kubectl config current-context)
info "Current cluster context: ${CURRENT_CONTEXT}"

# Handle Kind cluster creation (only for Kind mode)
if [[ "$DEPLOY_MODE" == "kind" ]]; then
    # Check if cluster already exists
    CLUSTER_EXISTS=false
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        CLUSTER_EXISTS=true
    fi

    if [[ "$CLUSTER_EXISTS" == true ]]; then
        warn "Kind cluster '${CLUSTER_NAME}' already exists."
        read -p "Do you want to delete and recreate it? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            info "Deleting existing cluster..."
            kind delete cluster --name "$CLUSTER_NAME"
            info "Creating new Kind cluster '${CLUSTER_NAME}'..."
            kind create cluster --name "$CLUSTER_NAME"
        else
            info "Using existing cluster..."
        fi
    else
        # Create Kind cluster if it doesn't exist
        info "Kind cluster '${CLUSTER_NAME}' not found. Creating new cluster..."
        kind create cluster --name "$CLUSTER_NAME"
    fi

    # Set kubectl context to the Kind cluster
    kubectl config use-context "kind-${CLUSTER_NAME}" || {
        warn "Could not switch to kind context, continuing with current context..."
    }

    # Wait for cluster to be ready
    info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=90s || {
        warn "Cluster nodes may not be fully ready, continuing anyway..."
    }
fi

# Build operator image (if requested)
if [[ "$BUILD_IMAGE" == true ]]; then
    info "Building operator image..."
    cd "$PROJECT_ROOT"
    make manifests generate
    make docker-build IMG="$IMAGE_NAME" CONTAINER_TOOL="$CONTAINER_TOOL"

    # Push image if requested
    if [[ "$PUSH_IMAGE" == true ]]; then
        info "Pushing operator image to registry..."
        make docker-push IMG="$IMAGE_NAME" CONTAINER_TOOL="$CONTAINER_TOOL"
    elif [[ "$DEPLOY_MODE" == "kind" ]]; then
        # Load image into Kind cluster (only for Kind, not for existing clusters)
        info "Loading operator image into Kind cluster..."
        if [[ "$CONTAINER_TOOL" == "podman" ]]; then
            # Podman requires exporting the image first, then loading it
            podman save "$IMAGE_NAME" -o /tmp/maas-operator-image.tar
            kind load image-archive /tmp/maas-operator-image.tar --name "$CLUSTER_NAME"
            rm -f /tmp/maas-operator-image.tar
        else
            kind load docker-image "$IMAGE_NAME" --name "$CLUSTER_NAME"
        fi
    fi
else
    info "Skipping image build (using existing image: $IMAGE_NAME)"
fi

# Install CRDs
info "Installing CRDs..."
cd "$PROJECT_ROOT"
make install

# Deploy operator
info "Deploying operator..."
make deploy IMG="$IMAGE_NAME"

# Wait for operator to be ready
info "Waiting for operator to be ready..."
kubectl wait --for=condition=Available deployment/controller-manager -n maas-operator-system --timeout=120s || {
    warn "Operator deployment may still be starting. Check status with: kubectl get pods -n maas-operator-system"
}

info "✅ Deployment complete!"
info ""
if [[ "$DEPLOY_MODE" == "kind" ]]; then
    info "Deployment mode: Kind cluster"
    info "Cluster name: ${CLUSTER_NAME}"
    info "To clean up, run:"
    info "  kind delete cluster --name ${CLUSTER_NAME}"
else
    info "Deployment mode: Existing cluster"
    info "Cluster context: ${CURRENT_CONTEXT}"
fi
info "Image: ${IMAGE_NAME}"
info ""
info "Next steps:"
info "  1. Create a MaasPlatform resource:"
info "     kubectl apply -k config/samples/"
info ""
info "  2. Check operator status:"
info "     kubectl get pods -n maas-operator-system"
info ""
info "  3. View operator logs:"
info "     kubectl logs -n maas-operator-system deployment/controller-manager"

