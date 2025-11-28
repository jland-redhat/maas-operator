# Deploying MaasPlatform and Tier Resources

This guide explains how to deploy and configure MaasPlatform and Tier resources using the maas-operator.

## Prerequisites

Before deploying MaasPlatform resources, ensure you have:

1. **maas-operator installed** in your cluster
2. **Dependencies installed**:
   - OpenDataHub (ODH) with KServe enabled (for model serving)
   - Kuadrant, Authorino, and Limitador operators (for API gateway policies)
   - Gateway API support enabled
   - cert-manager (for certificate management)

### Quick Setup

Use the provided script to install test dependencies:

```bash
# Using Podman (recommended)
./scripts/setup-test-dependencies.sh

# Docker alternative:
CONTAINER_TOOL=docker ./scripts/setup-test-dependencies.sh
```

This script will install:
- cert-manager
- Kuadrant operators (Kuadrant, Authorino, Limitador)
- OpenDataHub operator and DataScienceCluster with KServe (OpenShift only)

For manual installation, see the full deployment script in `maas-billing/deployment/scripts/deploy-openshift.sh`.

3. **Environment Variable** (optional):
   - `MAAS_DEPLOYMENT_BASE`: Path to `maas-billing/deployment/base` directory
     - If not set, the operator will attempt to discover it automatically

## Deploying MaasPlatform

The MaasPlatform resource orchestrates the deployment of the MaaS infrastructure components.

### Basic MaasPlatform

```yaml
apiVersion: myapp.io.odh.maas/v1alpha1
kind: MaasPlatform
metadata:
  name: maas-platform
  namespace: maas-system
spec: {}
```

### What Gets Deployed

When you create a MaasPlatform resource, the operator automatically deploys:

1. **MaaS API Components** (`deployment/base/maas-api`):
   - Deployment
   - Service
   - ServiceAccount
   - ClusterRole and ClusterRoleBinding
   - HTTPRoute
   - AuthPolicy (for API authentication)
   - **Note**: The `tier-to-group-mapping` ConfigMap is **not** deployed here (it's managed by Tier resources)

2. **Networking Components** (`deployment/base/networking`):
   - GatewayClass (openshift-default)
   - Gateway (maas-default-gateway)
   - Kuadrant instance

3. **Gateway Auth Policy** (`deployment/base/policies/gateway-auth-policy.yaml`):
   - Gateway-level authentication policy
   - Tier metadata lookup configuration
   - OpenShift identity authentication

### Verification

After deploying MaasPlatform, verify the deployment:

```bash
# Check MaasPlatform status
kubectl get maasplatform -n maas-system

# Check deployed resources
kubectl get pods -n maas-api
kubectl get gateway -n openshift-ingress maas-default-gateway
kubectl get kuadrant -n kuadrant-system

# Check operator logs
kubectl logs -n maas-operator-system deployment/controller-manager -c manager
```

## Deploying Tier Resources

Tier resources define rate limiting policies and model access controls for your MaasPlatform.

### Basic Tier Example

```yaml
apiVersion: myapp.io.odh.maas/v1alpha1
kind: Tier
metadata:
  name: free-tier
  namespace: maas-system
spec:
  targetRef:
    name: maas-platform
    namespace: maas-system
  rateLimits:
    limit: 5
    window: "2m"
  tokenRateLimits:
    limit: 100
    window: "1m"
  models:
    - "facebook/opt-125m"
    - "gpt-3.5-turbo"
```

### Tier Configuration Fields

- **targetRef**: References the MaasPlatform this tier applies to
  - `name`: Name of the MaasPlatform resource (required)
  - `namespace`: Namespace of the MaasPlatform (optional, defaults to Tier's namespace)

- **rateLimits**: HTTP request rate limiting
  - `limit`: Maximum number of requests allowed (required)
  - `window`: Time window for the limit (e.g., "2m", "1h", "30s") (required)
  - `counters`: Counter expressions for tracking (optional, default: ["auth.identity.userid"])

- **tokenRateLimits**: Token-based rate limiting (from model responses)
  - `limit`: Maximum number of tokens allowed (required)
  - `window`: Time window for the limit (required)
  - `counters`: Counter expressions for tracking (optional, default: ["auth.identity.userid"])

- **models**: List of model names this tier applies to
  - If empty or not specified: applies to all models
  - If specified: only these models are affected by this tier's rate limits

### Multiple Tiers Example

You can create multiple tiers for different user groups:

```yaml
---
apiVersion: myapp.io.odh.maas/v1alpha1
kind: Tier
metadata:
  name: free-tier
  namespace: maas-system
spec:
  targetRef:
    name: maas-platform
  rateLimits:
    limit: 5
    window: "2m"
  tokenRateLimits:
    limit: 100
    window: "1m"
  models:
    - "facebook/opt-125m"
---
apiVersion: myapp.io.odh.maas/v1alpha1
kind: Tier
metadata:
  name: premium-tier
  namespace: maas-system
spec:
  targetRef:
    name: maas-platform
  rateLimits:
    limit: 20
    window: "2m"
  tokenRateLimits:
    limit: 50000
    window: "1m"
  models:
    - "facebook/opt-125m"
    - "gpt-3.5-turbo"
    - "gpt-4"
---
apiVersion: myapp.io.odh.maas/v1alpha1
kind: Tier
metadata:
  name: enterprise-tier
  namespace: maas-system
spec:
  targetRef:
    name: maas-platform
  rateLimits:
    limit: 50
    window: "2m"
  tokenRateLimits:
    limit: 100000
    window: "1m"
  # No models specified = applies to all models
```

### What Gets Updated

When you create or update Tier resources, the operator automatically:

1. **Updates ConfigMap** (`tier-to-group-mapping` in `maas-api` namespace):
   - Aggregates all Tiers targeting the MaasPlatform
   - Generates tier mapping configuration
   - Maps tier names to user groups

2. **Updates RateLimitPolicy** (`gateway-rate-limits` in `openshift-ingress` namespace):
   - Combines rate limits from all Tiers
   - Creates tier-based rate limit rules
   - Each tier gets its own limit rule with predicate matching

3. **Updates TokenRateLimitPolicy** (`gateway-token-rate-limits` in `openshift-ingress` namespace):
   - Combines token rate limits from all Tiers
   - Creates tier-based token limit rules

### Verification

After deploying Tier resources, verify the updates:

```bash
# Check Tier resources
kubectl get tiers -n maas-system

# Check ConfigMap
kubectl get configmap tier-to-group-mapping -n maas-api -o yaml

# Check RateLimitPolicy
kubectl get ratelimitpolicy gateway-rate-limits -n openshift-ingress -o yaml

# Check TokenRateLimitPolicy
kubectl get tokenratelimitpolicy gateway-token-rate-limits -n openshift-ingress -o yaml

# Check operator logs
kubectl logs -n maas-operator-system deployment/controller-manager -c manager | grep -i tier
```

## Deployment Flow

1. **Deploy MaasPlatform** → Operator deploys infrastructure
2. **Deploy Tiers** → Operator updates ConfigMap and rate limit policies
3. **Verify** → Check resources and operator logs

## Troubleshooting

### MaasPlatform Not Deploying Resources

- Check if `MAAS_DEPLOYMENT_BASE` environment variable is set correctly
- Verify the maas-billing deployment directory is accessible
- Check operator logs for deployment errors:
  ```bash
  kubectl logs -n maas-operator-system deployment/controller-manager -c manager
  ```

### Tier Policies Not Updating

- Verify Tier resource targets the correct MaasPlatform
- Check if RateLimitPolicy and TokenRateLimitPolicy CRDs are installed
- Ensure Kuadrant operators are running:
  ```bash
  kubectl get pods -n kuadrant-system
  ```

### ConfigMap Not Updating

- Verify Tier resources exist and are targeting the MaasPlatform
- Check operator logs for Tier reconciliation errors
- Verify ConfigMap namespace matches (default: `maas-api`)

## Sample Resources

Sample resource manifests are available in `config/samples/`:

```bash
# Deploy sample MaasPlatform
kubectl apply -f config/samples/myapp_v1alpha1_maasplatform.yaml

# Deploy sample Tier
kubectl apply -f config/samples/myapp_v1alpha1_tier.yaml

# Or apply all samples at once
kubectl apply -k config/samples/
```

## Next Steps

After deploying MaasPlatform and Tier resources:

1. Deploy models using KServe LLMInferenceService
2. Test API endpoints with proper authentication
3. Verify rate limiting is working as expected
4. Monitor metrics and observability

