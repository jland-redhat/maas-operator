# Operator Reconciler Drift Detection Example

This example demonstrates how the **maas-operator** uses Kubernetes operator patterns to ensure that deployed resources remain consistent with the desired state defined in the MaasPlatform Custom Resource (CR).

## Operator Context

The maas-operator is a Kubernetes operator built with Kubebuilder/controller-runtime that:
- **Watches** MaasPlatform Custom Resources
- **Reconciles** the cluster state to match the desired state
- **Manages** Gateway, Deployment, Service, and other Kubernetes resources
- **Self-heals** by detecting and correcting drift

## Scenario

A MaasPlatform Custom Resource specifies a gateway hostname in its spec, but someone manually changes the Gateway resource in the cluster. The operator's reconciliation loop detects this drift and automatically reverts the change.

## Initial State

### MaasPlatform Resource

```yaml
apiVersion: myapp.io.odh.maas/v1alpha1
kind: MaasPlatform
metadata:
  name: maas-platform
  namespace: maas-system
spec:
  gatewayConfig:
    hostname: "my-maas-hostname.com"
```

### Desired Gateway Resource (from deployment manifests)

The operator reads the Gateway manifest from `deployment/base/networking/gateway.yaml` and applies it. The desired state includes:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: maas-default-gateway
  namespace: openshift-ingress
spec:
  gatewayClassName: openshift-default
  listeners:
  - name: https
    protocol: HTTPS
    port: 443
    hostname: "my-maas-hostname.com"  # ← Matches MaasPlatform spec
    allowedRoutes:
      namespaces:
        from: All
```

## Drift Detection Scenario

### Step 1: Someone Manually Changes the Gateway

An administrator (or another process) manually edits the Gateway resource and changes the hostname:

```bash
kubectl patch gateway maas-default-gateway -n openshift-ingress --type=json \
  -p='[{"op": "replace", "path": "/spec/listeners/0/hostname", "value": "custom-hostname.com"}]'
```

### Step 2: Current State (After Manual Change)

The Gateway now has the wrong hostname:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: maas-default-gateway
  namespace: openshift-ingress
spec:
  gatewayClassName: openshift-default
  listeners:
  - name: https
    protocol: HTTPS
    port: 443
    hostname: "custom-hostname.com"  # ← WRONG! Doesn't match desired state
    allowedRoutes:
      namespaces:
        from: All
```

### Step 3: Operator Reconciler Detects Drift

The operator's reconciliation loop is triggered (either by a watch event on the Gateway resource or periodic reconciliation). The `MaasPlatformReconciler.Reconcile()` method executes:

1. **Fetches the MaasPlatform CR** from the API server to get the desired hostname: `"my-maas-hostname.com"`
2. **Reads the deployment manifest** from `deployment/base/networking/gateway.yaml` (the source of truth)
3. **Fetches the current Gateway** from the Kubernetes API server
4. **Compares** the current cluster state with the desired state
5. **Detects the mismatch**: `custom-hostname.com` ≠ `my-maas-hostname.com`

### Step 4: Operator Reconciles the Change

The operator's reconciler applies the correct configuration using Kubernetes client-go:

```go
// Actual reconciler logic from maasplatform_controller.go

func (r *MaasPlatformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)
    
    // 1. Fetch the MaasPlatform Custom Resource
    maasPlatform := &myappv1alpha1.MaasPlatform{}
    if err := r.Get(ctx, req.NamespacedName, maasPlatform); err != nil {
        return ctrl.Result{}, err
    }
    
    // 2. Read deployment manifest (source of truth)
    gatewayManifest := readManifest("deployment/base/networking/gate  way.yaml")
    
    // 3. Apply overrides from MaasPlatform spec
    if maasPlatform.Spec.GatewayConfig.Hostname != "" {
        gatewayManifest.SetHostname(maasPlatform.Spec.GatewayConfig.Hostname)
    }
    
    // 4. Apply using server-side apply semantics
    // This will update the Gateway if it has drifted
    if err := r.applyUnstructured(ctx, gatewayManifest); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}

// applyUnstructured handles create/update logic
func (r *MaasPlatformReconciler) applyUnstructured(ctx context.Context, obj *unstructured.Unstructured) error {
    current := &unstructured.Unstructured{}
    current.SetGroupVersionKind(obj.GroupVersionKind())
    
    key := client.ObjectKeyFromObject(obj)
    err := r.Get(ctx, key, current)
    
    if errors.IsNotFound(err) {
        // Create new resource
        return r.Create(ctx, obj)
    } else if err != nil {
        return err
    }
    
    // Update existing resource - this corrects any drift
    obj.SetResourceVersion(current.GetResourceVersion())
    return r.Update(ctx, obj)  // ← This reverts the manual change
}
```

### Step 5: Gateway Reverted to Correct State

After reconciliation, the Gateway is restored to the correct configuration:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: maas-default-gateway
  namespace: openshift-ingress
spec:
  gatewayClassName: openshift-default
  listeners:
  - name: https
    protocol: HTTPS
    port: 443
    hostname: "my-maas-hostname.com"  # ← CORRECTED! Matches desired state
    allowedRoutes:
      namespaces:
        from: All
```

## How the Operator Works

The maas-operator follows standard Kubernetes operator patterns:

### 1. Controller Setup

The operator registers watches in `SetupWithManager()`:

```go
func (r *MaasPlatformReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&myappv1alpha1.MaasPlatform{}).  // Watch MaasPlatform CRs
        Named("maasplatform").
        Complete(r)
}
```

### 2. Reconciliation Loop

The operator's reconciliation loop follows this pattern:

1. **Watch Events**: The controller-runtime watches MaasPlatform CRs and triggers reconciliation on:
   - Create/Update/Delete events on MaasPlatform resources
   - Periodic reconciliation (configurable)

2. **Desired State**: On each reconciliation:
   - Reads the MaasPlatform CR spec (desired state)
   - Reads deployment manifests (source of truth)
   - Merges spec overrides into manifests

3. **Current State**: Fetches existing resources from the Kubernetes API server

4. **Drift Detection**: Compares desired vs. current state

5. **Correction**: When drift is detected:
   - Uses `client.Update()` to apply the correct configuration
   - Maintains resource version for optimistic concurrency
   - Ensures idempotency (safe to run multiple times)

### 3. Owner References

All managed resources have owner references pointing to the MaasPlatform CR:

```go
ctrl.SetControllerReference(maasPlatform, obj, r.Scheme)
```

This ensures:
- **Garbage Collection**: If MaasPlatform is deleted, all managed resources are automatically deleted
- **Resource Management**: Kubernetes knows which resources belong to which MaasPlatform instance

### 4. Server-Side Apply Semantics

The operator uses standard Kubernetes client operations:
- `Create()` for new resources
- `Update()` for existing resources (corrects drift)
- Maintains resource version to handle concurrent updates

## Operator Key Concepts

- **Idempotent Reconciliation**: The reconciler can run multiple times safely - it always converges to the desired state defined in the CR spec
- **Declarative Management**: The desired state is declared in the MaasPlatform CR spec, not hardcoded in the operator
- **Self-Healing**: Any manual changes to managed resources are automatically reverted on the next reconciliation cycle
- **Controller Pattern**: Follows the standard Kubernetes controller pattern: watch → reconcile → update
- **Resource Ownership**: All managed resources are owned by the MaasPlatform CR via owner references
- **Convergence**: The operator continuously drives the cluster state toward the desired state

## Example Operator Log Output

When the operator detects and corrects drift, you'll see logs from the operator pod:

```bash
# View operator logs
kubectl logs -n maas-operator-system deployment/controller-manager -c manager
```

Example log output:
```
INFO    Reconciling MaasPlatform    {"namespace": "maas-system", "name": "maas-platform"}
INFO    Deploying networking resources
INFO    Successfully deployed resource    {"kind": "Gateway", "name": "maas-default-gateway", "namespace": "openshift-ingress"}
INFO    Updated Gateway maas-default-gateway to match desired state    {"oldHostname": "custom-hostname.com", "newHostname": "my-maas-hostname.com"}
INFO    Reconciliation complete    {"namespace": "maas-system", "name": "maas-platform"}
```

## Operator Deployment

The operator itself runs as a Deployment in the cluster:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: maas-operator-system
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: manager
        image: quay.io/maas/maas-operator:latest
        # The operator process runs the reconciliation loop
```

The operator continuously watches for changes and reconciles resources to match the desired state defined in MaasPlatform Custom Resources.

