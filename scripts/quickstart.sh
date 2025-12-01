#!/usr/bin/env bash

# Quickstart script for maas-operator
# This script can either create a Kind cluster or deploy to an existing Kubernetes/OpenShift cluster

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default values
CLUSTER_NAME="${KIND_CLUSTER:-maas-operator}"
IMAGE_NAME="${IMG:-controller:latest}"
BUNDLE_IMAGE_NAME="${BUNDLE_IMG:-}"
CATALOG_IMAGE_NAME="${CATALOG_IMG:-}"
CONTAINER_TOOL="${CONTAINER_TOOL:-podman}"
DEPLOY_MODE="kind"  # "kind", "cluster", or "olm"
BUILD_IMAGE=true
PUSH_IMAGE=false
VERSION="${VERSION:-0.0.1}"
INSTALL_OPERATOR=false  # For OLM mode: actually install the operator via Subscription
OPERATOR_NAMESPACE="openshift-operators"  # Default to all namespaces installation

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

Deployment Modes:
    --kind                    Deploy to a Kind cluster (default, direct deployment)
    --cluster                 Deploy to an existing Kubernetes/OpenShift cluster (direct deployment)
    --olm                     Deploy to OpenShift using OLM (Operator Lifecycle Manager)
                             This will create a CatalogSource and install via OpenShift console

Options:
    --image IMAGE             Operator image name (default: controller:latest)
    --bundle-image IMAGE      Bundle image name (for OLM only, default: \${IMAGE}-bundle:v\${VERSION})
    --catalog-image IMAGE     Catalog image name (for OLM only, default: \${IMAGE}-catalog:v\${VERSION})
    --version VERSION         Operator version (default: 0.0.1)
    --install                 (OLM only) Automatically install the operator via Subscription
    --namespace NAMESPACE     (OLM only) Namespace to install operator (default: openshift-operators for all namespaces)
    --no-build                Skip building the image (use existing image)
    --push                    Push image to registry (required for existing clusters and OLM)
    --container-tool TOOL      Container tool to use: podman or docker (default: podman)
    -h, --help               Show this help message

Environment Variables:
    KIND_CLUSTER              Kind cluster name (default: maas-operator)
    IMG                       Operator image name (default: controller:latest)
    BUNDLE_IMG                Bundle image name (for OLM)
    CATALOG_IMG               Catalog image name (for OLM)
    VERSION                   Operator version (default: 0.0.1)
    CONTAINER_TOOL            Container tool: podman or docker (default: podman)

Examples:
    # Deploy to a new Kind cluster (default)
    $0

    # Deploy to existing Kubernetes cluster with image push
    $0 --cluster --image quay.io/maas/maas-operator:v0.1.0 --push

    # Deploy to existing cluster without building (image already exists)
    $0 --cluster --image quay.io/maas/maas-operator:v0.1.0 --no-build

    # Deploy to OpenShift using OLM (shows up in Installed Operators)
    $0 --olm --image quay.io/maas/maas-operator:v0.1.0 --version 0.1.0 --push

    # Deploy to OpenShift using OLM and auto-install via Subscription
    $0 --olm --image quay.io/maas/maas-operator:v0.1.0 --version 0.1.0 --push --install

    # Deploy to OpenShift using OLM with existing images
    $0 --olm --image quay.io/maas/maas-operator:v0.1.0 --version 0.1.0 --no-build

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
        --olm)
            DEPLOY_MODE="olm"
            shift
            ;;
        --image)
            IMAGE_NAME="$2"
            shift 2
            ;;
        --bundle-image)
            BUNDLE_IMAGE_NAME="$2"
            shift 2
            ;;
        --catalog-image)
            CATALOG_IMAGE_NAME="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --install)
            INSTALL_OPERATOR=true
            shift
            ;;
        --namespace)
            OPERATOR_NAMESPACE="$2"
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

# Set default bundle and catalog images if not provided
if [[ -z "$BUNDLE_IMAGE_NAME" ]]; then
    # Extract base from operator image
    IMAGE_BASE="${IMAGE_NAME%:*}"
    BUNDLE_IMAGE_NAME="${IMAGE_BASE}-bundle:v${VERSION}"
fi

if [[ -z "$CATALOG_IMAGE_NAME" ]]; then
    IMAGE_BASE="${IMAGE_NAME%:*}"
    CATALOG_IMAGE_NAME="${IMAGE_BASE}-catalog:v${VERSION}"
fi

# Validate deployment mode
if [[ "$DEPLOY_MODE" == "olm" ]]; then
    if [[ "$PUSH_IMAGE" == false ]] && [[ "$BUILD_IMAGE" == true ]]; then
        error "OLM deployment requires --push to push images to a registry."
        error "Images must be accessible from the cluster."
        exit 1
    fi
    if [[ "$BUILD_IMAGE" == false ]] && [[ "$PUSH_IMAGE" == true ]]; then
        error "Cannot push without building. Use --no-build without --push if images already exist."
        exit 1
    fi
elif [[ "$DEPLOY_MODE" == "cluster" ]] && [[ "$BUILD_IMAGE" == true ]] && [[ "$PUSH_IMAGE" == false ]]; then
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

if [[ "$DEPLOY_MODE" == "olm" ]]; then
    check_command operator-sdk
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

# Check if this is an OpenShift cluster
IS_OPENSHIFT=false
if kubectl get project default &> /dev/null || kubectl api-resources | grep -q "route.openshift.io"; then
    IS_OPENSHIFT=true
    info "Detected OpenShift cluster"
fi

# Validate OLM mode is used with OpenShift
if [[ "$DEPLOY_MODE" == "olm" ]] && [[ "$IS_OPENSHIFT" == false ]]; then
    warn "OLM deployment is optimized for OpenShift clusters."
    warn "For vanilla Kubernetes, consider using --cluster mode instead."
    read -p "Continue with OLM deployment anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 0
    fi
fi

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

# Deploy based on mode
if [[ "$DEPLOY_MODE" == "olm" ]]; then
    info "Deploying operator using OLM (Operator Lifecycle Manager)..."
    
    # Check if OLM is installed
    info "Checking if OLM is installed..."
    if ! kubectl get crd catalogsources.operators.coreos.com &>/dev/null; then
        error "OLM (Operator Lifecycle Manager) is not installed on this cluster."
        echo ""
        info "OLM is required for --olm mode. You have two options:"
        echo ""
        echo "1. Install OLM on your cluster:"
        echo "   operator-sdk olm install"
        echo ""
        echo "2. Use direct deployment instead (won't show in OLM console):"
        echo "   ./scripts/quickstart.sh --cluster --image $IMAGE_NAME --push"
        echo ""
        
        # Offer to install OLM automatically
        read -p "Would you like to install OLM now? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            info "Installing OLM..."
            operator-sdk olm install || {
                error "Failed to install OLM. Please install it manually."
                exit 1
            }
            info "✅ OLM installed successfully"
        else
            info "Aborting. Please install OLM or use --cluster mode instead."
            exit 1
        fi
    else
        info "✅ OLM is installed"
    fi
    
    # Determine the correct namespace for CatalogSource
    CATALOG_NAMESPACE="olm"
    if kubectl get namespace openshift-marketplace &> /dev/null; then
        CATALOG_NAMESPACE="openshift-marketplace"
    fi
    info "Using CatalogSource namespace: ${CATALOG_NAMESPACE}"
    
    cd "$PROJECT_ROOT"
    
    # Generate bundle
    info "Generating OLM bundle..."
    make bundle IMG="$IMAGE_NAME" VERSION="$VERSION"
    
    # Build and push bundle image if building
    if [[ "$BUILD_IMAGE" == true ]]; then
        info "Building bundle image..."
        make bundle-build BUNDLE_IMG="$BUNDLE_IMAGE_NAME" CONTAINER_TOOL="$CONTAINER_TOOL"
        
        if [[ "$PUSH_IMAGE" == true ]]; then
            info "Pushing bundle image..."
            make bundle-push BUNDLE_IMG="$BUNDLE_IMAGE_NAME" CONTAINER_TOOL="$CONTAINER_TOOL"
        fi
        
        # Build and push catalog image
        info "Building catalog image..."
        make catalog-build CATALOG_IMG="$CATALOG_IMAGE_NAME" BUNDLE_IMGS="$BUNDLE_IMAGE_NAME" CONTAINER_TOOL="$CONTAINER_TOOL"
        
        if [[ "$PUSH_IMAGE" == true ]]; then
            info "Pushing catalog image..."
            make catalog-push CATALOG_IMG="$CATALOG_IMAGE_NAME" CONTAINER_TOOL="$CONTAINER_TOOL"
        elif [[ "$DEPLOY_MODE" == "kind" ]]; then
            # Load catalog into Kind if not pushing
            info "Loading catalog image into Kind cluster..."
            if [[ "$CONTAINER_TOOL" == "podman" ]]; then
                podman save "$CATALOG_IMAGE_NAME" -o /tmp/maas-operator-catalog.tar
                kind load image-archive /tmp/maas-operator-catalog.tar --name "$CLUSTER_NAME"
                rm -f /tmp/maas-operator-catalog.tar
            else
                kind load docker-image "$CATALOG_IMAGE_NAME" --name "$CLUSTER_NAME"
            fi
        fi
    fi
    
    # Create CatalogSource
    info "Creating CatalogSource in ${CATALOG_NAMESPACE}..."
    cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: maas-operator-catalog
  namespace: ${CATALOG_NAMESPACE}
spec:
  sourceType: grpc
  image: ${CATALOG_IMAGE_NAME}
  displayName: MaaS Operator
  publisher: Red Hat
  updateStrategy:
    registryPoll:
      interval: 10m
EOF
    
    info "Waiting for CatalogSource to be ready..."
    kubectl wait --for=jsonpath='{.status.connectionState.lastObservedState}'=READY \
        catalogsource/maas-operator-catalog -n "${CATALOG_NAMESPACE}" --timeout=120s || {
        warn "CatalogSource may still be initializing..."
    }
    
    info "✅ OLM Bundle deployment complete!"
    info ""
    info "Deployment mode: OLM (Operator Lifecycle Manager)"
    info "Cluster context: ${CURRENT_CONTEXT}"
    info "Operator image: ${IMAGE_NAME}"
    info "Bundle image: ${BUNDLE_IMAGE_NAME}"
    info "Catalog image: ${CATALOG_IMAGE_NAME}"
    info "CatalogSource namespace: ${CATALOG_NAMESPACE}"
    info ""
    
    # Optionally install the operator via Subscription
    if [[ "$INSTALL_OPERATOR" == true ]]; then
        info "Installing operator via Subscription..."
        
        # Create namespace if it doesn't exist and it's not openshift-operators
        if [[ "$OPERATOR_NAMESPACE" != "openshift-operators" ]]; then
            kubectl create namespace "$OPERATOR_NAMESPACE" 2>/dev/null || true
            info "Created namespace: ${OPERATOR_NAMESPACE}"
        fi
        
        # Create OperatorGroup if not in openshift-operators
        if [[ "$OPERATOR_NAMESPACE" != "openshift-operators" ]]; then
            info "Creating OperatorGroup..."
            cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: maas-operator-group
  namespace: ${OPERATOR_NAMESPACE}
spec:
  targetNamespaces:
  - ${OPERATOR_NAMESPACE}
EOF
        fi
        
        # Create Subscription
        info "Creating Subscription..."
        cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: maas-operator
  namespace: ${OPERATOR_NAMESPACE}
spec:
  channel: alpha
  name: maas-operator
  source: maas-operator-catalog
  sourceNamespace: ${CATALOG_NAMESPACE}
  installPlanApproval: Automatic
EOF
        
        # Wait for CSV to be created
        info "Waiting for ClusterServiceVersion to be created..."
        for i in {1..30}; do
            if kubectl get csv -n "$OPERATOR_NAMESPACE" 2>/dev/null | grep -q "maas-operator"; then
                break
            fi
            sleep 2
        done
        
        # Check for pending install plans that need approval
        info "Checking for install plans..."
        INSTALL_PLAN=$(kubectl get installplan -n "$OPERATOR_NAMESPACE" -o json 2>/dev/null | \
            jq -r '.items[] | select(.spec.clusterServiceVersionNames[] | contains("maas-operator")) | select(.spec.approved == false) | .metadata.name' | head -1)
        
        if [[ -n "$INSTALL_PLAN" ]]; then
            info "Approving install plan: ${INSTALL_PLAN}"
            kubectl patch installplan "$INSTALL_PLAN" -n "$OPERATOR_NAMESPACE" --type merge -p '{"spec":{"approved":true}}'
            sleep 3
        fi
        
        # Wait for CSV to be ready
        info "Waiting for operator to be ready..."
        CSV_NAME=$(kubectl get csv -n "$OPERATOR_NAMESPACE" -o name 2>/dev/null | grep maas-operator | head -1)
        if [[ -n "$CSV_NAME" ]]; then
            kubectl wait --for=jsonpath='{.status.phase}'=Succeeded \
                "$CSV_NAME" -n "$OPERATOR_NAMESPACE" --timeout=120s || {
                warn "Operator may still be installing..."
            }
            
            info "✅ Operator installed successfully!"
            info ""
            info "Operator details:"
            kubectl get csv -n "$OPERATOR_NAMESPACE" | grep maas-operator
            info ""
            info "Operator pods:"
            kubectl get pods -n "$OPERATOR_NAMESPACE" | grep maas-operator
        else
            warn "Could not find CSV. Check status with: kubectl get csv -n ${OPERATOR_NAMESPACE}"
        fi
        
        info ""
        info "Next steps:"
        info "  1. Create a MaasPlatform resource:"
        info "     kubectl apply -f config/samples/myapp_v1alpha1_maasplatform.yaml"
        info ""
        info "  2. View operator in OpenShift Console:"
        info "     Operators → Installed Operators → MaaS Operator"
        info ""
        info "  3. To uninstall:"
        info "     kubectl delete subscription maas-operator -n ${OPERATOR_NAMESPACE}"
        info "     kubectl delete csv -n ${OPERATOR_NAMESPACE} \$(kubectl get csv -n ${OPERATOR_NAMESPACE} -o name | grep maas-operator)"
    else
        info "Next steps:"
        if [[ "$IS_OPENSHIFT" == true ]]; then
            info "  1. Install the operator from OpenShift Console:"
            info "     - Navigate to: Operators → OperatorHub"
            info "     - Search for: 'MaaS Operator'"
            info "     - Click 'Install' and follow the wizard"
            info ""
        fi
        info "  Install via CLI:"
        info "     kubectl create namespace maas-operator-system"
        info "     cat <<YAML | kubectl apply -f -"
        info "apiVersion: operators.coreos.com/v1alpha1"
        info "kind: Subscription"
        info "metadata:"
        info "  name: maas-operator"
        info "  namespace: maas-operator-system"
        info "spec:"
        info "  channel: alpha"
        info "  name: maas-operator"
        info "  source: maas-operator-catalog"
        info "  sourceNamespace: ${CATALOG_NAMESPACE}"
        info "YAML"
        info ""
        info "  Or use --install flag to auto-install:"
        info "     $0 --olm --image ${IMAGE_NAME} --version ${VERSION} --no-build --install"
    fi
    
    info ""
    info "  Upgrade existing installation:"
    info "     # Delete old CSV to trigger upgrade"
    info "     kubectl delete csv -n ${OPERATOR_NAMESPACE} -l operators.coreos.com/maas-operator.${OPERATOR_NAMESPACE}="
    info "     # Watch for new version to install"
    info "     kubectl get csv -n ${OPERATOR_NAMESPACE} -w"
    info ""
    info "  Verify installation:"
    info "     kubectl get csv -n ${OPERATOR_NAMESPACE}"
    info "     kubectl get pods -n ${OPERATOR_NAMESPACE}"
    info ""
    info "  To uninstall the CatalogSource:"
    info "     kubectl delete catalogsource maas-operator-catalog -n ${CATALOG_NAMESPACE}"
    
else
    # Direct deployment (kind or cluster mode)
    info "Deploying operator directly (non-OLM mode)..."
    
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
        info "Deployment mode: Kind cluster (direct deployment)"
        info "Cluster name: ${CLUSTER_NAME}"
        info "To clean up, run:"
        info "  kind delete cluster --name ${CLUSTER_NAME}"
    else
        info "Deployment mode: Existing cluster (direct deployment)"
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
fi

