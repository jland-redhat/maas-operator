/*
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
*/

package controller

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	myappv1alpha1 "github.com/jland-redhat/maas-operator.git/api/v1alpha1"
)

//go:embed manifests
var manifestsFS embed.FS

// MaasPlatformReconciler reconciles a MaasPlatform object
type MaasPlatformReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=maasplatforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=maasplatforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=maasplatforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces;services;configmaps;serviceaccounts;secrets;pods;endpoints;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts/token,verbs=create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;gatewayclasses;httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kuadrant.io,resources=kuadrants;authpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices;llminferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state specified by
// the MaasPlatform object.
func (r *MaasPlatformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the MaasPlatform instance
	maasPlatform := &myappv1alpha1.MaasPlatform{}
	if err := r.Get(ctx, req.NamespacedName, maasPlatform); err != nil {
		if errors.IsNotFound(err) {
			log.Info("MaasPlatform resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get MaasPlatform")
		return ctrl.Result{}, err
	}

	// Create required namespaces
	log.Info("Ensuring required namespaces exist")
	if err := r.ensureNamespaces(ctx); err != nil {
		log.Error(err, "Failed to create required namespaces")
		return ctrl.Result{}, err
	}

	// Deploy maas-api resources (excluding ConfigMap which will be managed by Tier)
	log.Info("Deploying maas-api resources")
	if err := r.deployEmbeddedManifest(ctx, "manifests/maas-api/resources.yaml", maasPlatform, true); err != nil {
		log.Error(err, "Failed to deploy maas-api resources")
		return ctrl.Result{}, err
	}

	// Deploy networking resources
	log.Info("Deploying networking resources")
	if err := r.deployEmbeddedManifest(ctx, "manifests/networking/resources.yaml", maasPlatform, false); err != nil {
		log.Error(err, "Failed to deploy networking resources")
		return ctrl.Result{}, err
	}

	// Deploy gateway-auth-policy
	log.Info("Deploying gateway-auth-policy")
	if err := r.deployEmbeddedManifest(ctx, "manifests/policies/gateway-auth-policy.yaml", maasPlatform, false); err != nil {
		log.Error(err, "Failed to deploy gateway-auth-policy")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, maasPlatform); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ensureNamespaces creates required namespaces if they don't exist
func (r *MaasPlatformReconciler) ensureNamespaces(ctx context.Context) error {
	requiredNamespaces := []string{
		"maas-api",
		"kuadrant-system",
		// Add more namespaces here if needed
	}

	for _, ns := range requiredNamespaces {
		namespace := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": ns,
				},
			},
		}

		err := r.Get(ctx, client.ObjectKey{Name: ns}, namespace)
		if errors.IsNotFound(err) {
			// Create the namespace
			if err := r.Create(ctx, namespace); err != nil {
				return fmt.Errorf("failed to create namespace %s: %w", ns, err)
			}
			logf.FromContext(ctx).Info("Created namespace", "namespace", ns)
		} else if err != nil {
			return fmt.Errorf("failed to check namespace %s: %w", ns, err)
		}
	}

	return nil
}

// deployEmbeddedManifest deploys a manifest from the embedded filesystem
func (r *MaasPlatformReconciler) deployEmbeddedManifest(ctx context.Context, path string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap bool) error {
	log := logf.FromContext(ctx)

	data, err := manifestsFS.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read embedded manifest %s: %w", path, err)
	}

	// Substitute environment variables
	dataStr := string(data)
	dataStr = r.substituteEnvVars(ctx, dataStr)
	data = []byte(dataStr)

	// Parse YAML (support multi-document YAML)
	documents, err := splitYAMLDocuments(data)
	if err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	for _, doc := range documents {
		if len(doc) == 0 {
			continue
		}

		var obj unstructured.Unstructured
		if err := yaml.Unmarshal(doc, &obj.Object); err != nil {
			log.Info("Skipping non-Kubernetes resource (failed to unmarshal)", "file", path)
			continue
		}

		// Skip ConfigMap if requested
		if skipConfigMap && obj.GetKind() == "ConfigMap" && obj.GetName() == "tier-to-group-mapping" {
			log.Info("Skipping tier-to-group-mapping ConfigMap (managed by Tier controller)")
			continue
		}

		// For PVCs, check if they already exist and skip update (PVCs are immutable)
		if obj.GetKind() == "PersistentVolumeClaim" {
			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(obj.GroupVersionKind())
			err := r.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
			if err == nil {
				log.Info("PersistentVolumeClaim already exists, skipping update (PVCs are immutable)",
					"name", obj.GetName(),
					"namespace", obj.GetNamespace())
				continue
			} else if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to check existing PVC %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			// If not found, continue to create it
		}

		// Set owner reference (skip for cluster-scoped resources and cross-namespace resources)
		// Owner references cannot span namespaces
		if obj.GetNamespace() != "" && obj.GetNamespace() == maasPlatform.Namespace {
			if err := ctrl.SetControllerReference(maasPlatform, &obj, r.Scheme); err != nil {
				log.Info("Failed to set owner reference, continuing anyway", "error", err)
			}
		} else if obj.GetNamespace() != "" {
			log.V(1).Info("Skipping owner reference for cross-namespace resource",
				"kind", obj.GetKind(),
				"name", obj.GetName(),
				"namespace", obj.GetNamespace(),
				"owner-namespace", maasPlatform.Namespace)
		}

		// Apply the resource
		if err := r.applyUnstructured(ctx, &obj); err != nil {
			return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}

		log.Info("Successfully deployed resource", "kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
	}

	return nil
}

// substituteEnvVars replaces ${VAR} style variables in the manifest
func (r *MaasPlatformReconciler) substituteEnvVars(ctx context.Context, content string) string {
	log := logf.FromContext(ctx)

	// Get CLUSTER_DOMAIN from cluster if not set
	clusterDomain := os.Getenv("CLUSTER_DOMAIN")
	if clusterDomain == "" {
		// Try to detect from cluster ingress
		var ingressConfig unstructured.Unstructured
		ingressConfig.SetAPIVersion("config.openshift.io/v1")
		ingressConfig.SetKind("Ingress")
		err := r.Get(ctx, client.ObjectKey{Name: "cluster"}, &ingressConfig)
		if err == nil {
			if domain, found, _ := unstructured.NestedString(ingressConfig.Object, "spec", "domain"); found {
				clusterDomain = domain
				log.Info("Detected cluster domain", "domain", clusterDomain)
			}
		}

		if clusterDomain == "" {
			clusterDomain = "apps.example.com"
			log.Info("Using default cluster domain", "domain", clusterDomain)
		}
	}

	// Replace variables
	content = strings.ReplaceAll(content, "${CLUSTER_DOMAIN}", clusterDomain)
	content = strings.ReplaceAll(content, "$CLUSTER_DOMAIN", clusterDomain)

	return content
}

// deployResourcesFromPath is kept for backwards compatibility but no longer used
func (r *MaasPlatformReconciler) deployResourcesFromPath(ctx context.Context, path string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap bool) error {
	return fmt.Errorf("deprecated: use deployEmbeddedManifest instead")
}

// deployKustomizeDirectory is kept for backwards compatibility but no longer used
func (r *MaasPlatformReconciler) deployKustomizeDirectory(ctx context.Context, path string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap bool) error {
	return fmt.Errorf("deprecated: use deployEmbeddedManifest instead")
}

// deployDirectoryYAMLs is kept for backwards compatibility but no longer used
func (r *MaasPlatformReconciler) deployDirectoryYAMLs(ctx context.Context, dir string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap bool) error {
	return fmt.Errorf("deprecated: use deployEmbeddedManifest instead")
}

// deploySingleResource is kept for backwards compatibility but no longer used
func (r *MaasPlatformReconciler) deploySingleResource(ctx context.Context, filePath string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap ...bool) error {
	return fmt.Errorf("deprecated: use deployEmbeddedManifest instead")
}

// applyUnstructured applies an unstructured resource using server-side apply
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

	// Update existing resource - use server-side apply semantics
	obj.SetResourceVersion(current.GetResourceVersion())
	return r.Update(ctx, obj)
}

// updateStatus updates the MaasPlatform status
func (r *MaasPlatformReconciler) updateStatus(ctx context.Context, maasPlatform *myappv1alpha1.MaasPlatform) error {
	// Status updates can be added here when status fields are defined
	// For now, just update the status subresource
	return r.Status().Update(ctx, maasPlatform)
}

// splitYAMLDocuments splits multi-document YAML into individual documents
func splitYAMLDocuments(data []byte) ([][]byte, error) {
	var documents [][]byte
	var currentDoc []byte

	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("---")) && len(currentDoc) > 0 {
			documents = append(documents, currentDoc)
			currentDoc = nil
			continue
		}
		currentDoc = append(currentDoc, line...)
		if i < len(lines)-1 {
			currentDoc = append(currentDoc, '\n')
		}
	}
	if len(currentDoc) > 0 {
		documents = append(documents, currentDoc)
	}

	return documents, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MaasPlatformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&myappv1alpha1.MaasPlatform{}).
		Named("maasplatform").
		Complete(r)
}
