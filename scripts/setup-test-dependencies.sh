#!/usr/bin/env bash

# Setup Test Dependencies for maas-operator
# Installs ODH and Kuadrant/Limitador/Authorino operators for testing

set -euo pipefail

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

# Check if command exists
check_command() {
    if ! command -v "$1" &> /dev/null; then
        error "$1 is not installed. Please install it first."
        exit 1
    fi
}

echo "========================================="
echo "🚀 MaaS Operator Test Dependencies Setup"
echo "========================================="
echo ""

# Check prerequisites
info "Checking prerequisites..."
check_command kubectl

# Check if running on OpenShift
IS_OPENSHIFT=false
if kubectl api-resources | grep -q "route.openshift.io"; then
    IS_OPENSHIFT=true
    info "Detected OpenShift cluster"
    check_command git
    check_command kustomize || {
        warn "kustomize not found. ODH installation will attempt to use kubectl kustomize"
    }
else
    warn "Not running on OpenShift - some features may not work"
fi

echo ""
echo "1️⃣ Installing cert-manager..."

# Check if cert-manager is already installed
if kubectl get crd certificates.cert-manager.io &>/dev/null; then
    info "cert-manager is already installed"
else
    info "Installing cert-manager..."
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml || {
        error "cert-manager installation failed"
        exit 1
    }
fi

info "Waiting for cert-manager to be ready..."
kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s || \
    warn "cert-manager may still be starting"

echo ""
echo "2️⃣ Installing Kuadrant operators..."

# Ensure kuadrant-system namespace exists
kubectl create namespace kuadrant-system 2>/dev/null || info "Namespace kuadrant-system already exists"

# Create OperatorGroup
info "Creating Kuadrant OperatorGroup..."
kubectl apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: kuadrant-operator-group
  namespace: kuadrant-system
spec: {}
EOF

# Create CatalogSource if it doesn't exist
if kubectl get catalogsource kuadrant-operator-catalog -n kuadrant-system &>/dev/null; then
    info "Kuadrant CatalogSource already exists"
else
    info "Creating Kuadrant CatalogSource..."
    kubectl apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: kuadrant-operator-catalog
  namespace: kuadrant-system
spec:
  displayName: Kuadrant Operators
  grpcPodConfig:
    securityContextConfig: restricted
  image: 'quay.io/kuadrant/kuadrant-operator-catalog:v1.3.0'
  publisher: grpc
  sourceType: grpc
EOF
fi

# Create Subscription
info "Installing Kuadrant operators via OLM Subscription..."
kubectl apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: kuadrant-operator
  namespace: kuadrant-system
spec:
  channel: stable
  installPlanApproval: Automatic
  name: kuadrant-operator
  source: kuadrant-operator-catalog
  sourceNamespace: kuadrant-system
EOF

# Wait for operators to be ready
info "Waiting for Kuadrant operators to be ready (this may take a few minutes)..."
ATTEMPTS=0
MAX_ATTEMPTS=10
while true; do
    if kubectl get deployment/kuadrant-operator-controller-manager -n kuadrant-system &>/dev/null; then
        break
    else
        ATTEMPTS=$((ATTEMPTS+1))
        if [[ $ATTEMPTS -ge $MAX_ATTEMPTS ]]; then
            error "kuadrant-operator-controller-manager deployment not found after $MAX_ATTEMPTS attempts"
            exit 1
        fi
        info "Waiting for kuadrant-operator-controller-manager deployment... (attempt $ATTEMPTS/$MAX_ATTEMPTS)"
        sleep $((10 + 10 * $ATTEMPTS))
    fi
done

kubectl wait --for=condition=Available deployment/kuadrant-operator-controller-manager -n kuadrant-system --timeout=300s || \
    warn "Kuadrant operator may still be starting"
kubectl wait --for=condition=Available deployment/limitador-operator-controller-manager -n kuadrant-system --timeout=300s || \
    warn "Limitador operator may still be starting"
kubectl wait --for=condition=Available deployment/authorino-operator -n kuadrant-system --timeout=300s || \
    warn "Authorino operator may still be starting"

sleep 5

# Patch Kuadrant for OpenShift Gateway Controller (if on OpenShift)
if [[ "$IS_OPENSHIFT" == true ]]; then
    info "Patching Kuadrant operator for OpenShift Gateway Controller..."
    CSV_NAME=$(kubectl get csv -n kuadrant-system -o jsonpath='{.items[?(@.spec.displayName=="Kuadrant Operator")].metadata.name}' | head -n1)
    
    if [[ -n "$CSV_NAME" ]]; then
        if ! kubectl -n kuadrant-system get deployment kuadrant-operator-controller-manager -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="ISTIO_GATEWAY_CONTROLLER_NAMES")]}' | grep -q "ISTIO_GATEWAY_CONTROLLER_NAMES"; then
            kubectl patch csv "$CSV_NAME" -n kuadrant-system --type='json' -p='[
              {
                "op": "add",
                "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/env/-",
                "value": {
                  "name": "ISTIO_GATEWAY_CONTROLLER_NAMES",
                  "value": "istio.io/gateway-controller,openshift.io/gateway-controller/v1"
                }
              }
            ]' && info "✅ Kuadrant operator patched" || warn "Failed to patch Kuadrant operator"
        else
            info "✅ Kuadrant operator already configured"
        fi
    fi
fi

echo ""
echo "3️⃣ Installing OpenDataHub (ODH)..."

if [[ "$IS_OPENSHIFT" != true ]]; then
    warn "ODH installation on non-OpenShift clusters is not fully supported"
    warn "Skipping ODH installation"
else
    # Create opendatahub namespace
    kubectl create namespace opendatahub 2>/dev/null || info "Namespace opendatahub already exists"
    
    # Install ODH Operator (using dev install for now)
    info "Installing ODH Operator..."
    ODH_OPERATOR_NS="opendatahub-operator-system"
    kubectl create namespace $ODH_OPERATOR_NS 2>/dev/null || info "Namespace $ODH_OPERATOR_NS already exists"
    
    TMP_DIR=$(mktemp -d)
    trap 'rm -fr -- "$TMP_DIR"' EXIT
    
    pushd $TMP_DIR > /dev/null
    info "Cloning ODH operator repository..."
    git clone -q --depth 1 "https://github.com/opendatahub-io/opendatahub-operator.git" || {
        error "Failed to clone ODH operator repository"
        exit 1
    }
    
    pushd ./opendatahub-operator > /dev/null
    cp config/manager/kustomization.yaml.in config/manager/kustomization.yaml
    sed -i 's#REPLACE_IMAGE#quay.io/opendatahub/opendatahub-operator#' config/manager/kustomization.yaml
    
    info "Applying ODH operator manifests..."
    # Try kustomize first, fallback to kubectl kustomize
    if command -v kustomize &> /dev/null; then
        kustomize build --load-restrictor LoadRestrictionsNone config/default | kubectl apply --namespace $ODH_OPERATOR_NS -f -
    else
        kubectl kustomize --load-restrictor LoadRestrictionsNone config/default | kubectl apply --namespace $ODH_OPERATOR_NS -f -
    fi
    
    popd > /dev/null
    popd > /dev/null
    
    info "Waiting for ODH operator to be ready..."
    kubectl wait deployment/opendatahub-operator-controller-manager -n $ODH_OPERATOR_NS --for condition=Available=True --timeout=300s || \
        warn "ODH operator may still be starting"
    
    # Create DSCInitialization
    info "Creating DSCInitialization resource..."
    kubectl apply -f - <<EOF
apiVersion: dscinitialization.opendatahub.io/v2
kind: DSCInitialization
metadata:
  name: default-dsci
spec:
  applicationsNamespace: opendatahub
  monitoring:
    managementState: Managed
    namespace: opendatahub
    metrics: {}
  trustedCABundle:
    managementState: Managed
EOF
    
    info "Waiting for DSCInitialization to be ready..."
    for i in {1..30}; do
        if kubectl get dscinitializations -n opendatahub default-dsci -o jsonpath='{.status.phase}' 2>/dev/null | grep -q "Ready"; then
            info "✅ DSCInitialization is ready"
            break
        fi
        if [[ $i -eq 30 ]]; then
            warn "DSCInitialization did not become ready within timeout"
        fi
        sleep 10
    done
    
    # Create DataScienceCluster
    info "Creating DataScienceCluster..."
    kubectl apply -f - <<EOF
apiVersion: datasciencecluster.opendatahub.io/v2
kind: DataScienceCluster
metadata:
  name: default-dsc
spec:
  components:
    kserve:
      managementState: Managed
      nim:
        managementState: Managed
      rawDeploymentServiceConfig: Headless
      defaultDeploymentMode: RawDeployment
      serving:
        ingressGateway:
          certificate:
            type: OpenshiftDefaultIngress
        managementState: Removed
        name: knative-serving
    dashboard:
      managementState: Removed
    workbenches:
      managementState: Removed
    aipipelines:
      managementState: Removed
    ray:
      managementState: Removed
    kueue:
      managementState: Removed
    modelregistry:
      managementState: Removed
    trustyai:
      managementState: Removed
    trainingoperator:
      managementState: Removed
    feastoperator:
      managementState: Removed
    llamastackoperator:
      managementState: Removed
EOF
    
    info "Waiting for DataScienceCluster to be ready (this may take several minutes)..."
    for i in {1..60}; do
        PHASE=$(kubectl get datasciencecluster -n opendatahub default-dsc -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        CONDITIONS=$(kubectl get datasciencecluster -n opendatahub default-dsc -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
        
        if [[ "$PHASE" == "Ready" ]] || [[ "$CONDITIONS" == "True" ]]; then
            info "✅ DataScienceCluster is ready"
            break
        fi
        if [[ $i -eq 60 ]]; then
            warn "DataScienceCluster did not become ready within timeout"
        fi
        echo "   Status: Phase=$PHASE, Ready=$CONDITIONS ($i/60)"
        sleep 10
    done
fi

echo ""
echo "========================================="
echo "✅ Setup Complete!"
echo "========================================="
echo ""
echo "Installed components:"
echo "  ✅ cert-manager"
echo "  ✅ Kuadrant operators (Kuadrant, Authorino, Limitador)"
if [[ "$IS_OPENSHIFT" == true ]]; then
    echo "  ✅ OpenDataHub operator"
    echo "  ✅ DataScienceCluster with KServe"
fi
echo ""
echo "Next steps:"
echo "  1. Verify operator pods are running:"
echo "     kubectl get pods -n kuadrant-system"
if [[ "$IS_OPENSHIFT" == true ]]; then
    echo "     kubectl get pods -n opendatahub-operator-system"
fi
echo ""
echo "  2. Deploy maas-operator:"
echo "     make deploy IMG=quay.io/maas/maas-operator:latest"
echo ""
echo "  3. Create MaasPlatform and Tier resources:"
echo "     See docs/deployment.md for examples"
echo ""

