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
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	myappv1alpha1 "github.com/jland-redhat/maas-operator.git/api/v1alpha1"
)

// MaasPlatformReconciler reconciles a MaasPlatform object
type MaasPlatformReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=maasplatforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=maasplatforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=maasplatforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps;serviceaccounts;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways;gatewayclasses;httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kuadrant.io,resources=kuadrants;authpolicies,verbs=get;list;watch;create;update;patch;delete

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

	// Find the maas-billing deployment directory
	// The path should be set via environment variable or discovered relative to operator
	deploymentBase := os.Getenv("MAAS_DEPLOYMENT_BASE")
	if deploymentBase == "" {
		// Try multiple possible locations
		possiblePaths := []string{
			filepath.Join("..", "maas-billing", "deployment", "base"),
			filepath.Join("..", "..", "..", "maas-billing", "deployment", "base"),
			filepath.Join(".", "maas-billing", "deployment", "base"),
		}

		for _, path := range possiblePaths {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				deploymentBase = path
				break
			}
		}

		if deploymentBase == "" {
			return ctrl.Result{}, fmt.Errorf("cannot find deployment base directory. Set MAAS_DEPLOYMENT_BASE environment variable or ensure maas-billing/deployment/base exists relative to operator")
		}
	}

	// Deploy maas-api resources (excluding ConfigMap which will be managed by Tier)
	log.Info("Deploying maas-api resources")
	maasApiPath := filepath.Join(deploymentBase, "maas-api")
	if err := r.deployResourcesFromPath(ctx, maasApiPath, maasPlatform, true); err != nil {
		log.Error(err, "Failed to deploy maas-api resources")
		return ctrl.Result{}, err
	}

	// Deploy networking resources
	log.Info("Deploying networking resources")
	networkingPath := filepath.Join(deploymentBase, "networking")
	if err := r.deployResourcesFromPath(ctx, networkingPath, maasPlatform, false); err != nil {
		log.Error(err, "Failed to deploy networking resources")
		return ctrl.Result{}, err
	}

	// Deploy gateway-auth-policy
	log.Info("Deploying gateway-auth-policy")
	policyPath := filepath.Join(deploymentBase, "policies", "gateway-auth-policy.yaml")
	if err := r.deploySingleResource(ctx, policyPath, maasPlatform); err != nil {
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

// deployResourcesFromPath deploys resources from a kustomize directory or YAML file
func (r *MaasPlatformReconciler) deployResourcesFromPath(ctx context.Context, path string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap bool) error {

	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}

	// If it's a file, deploy it directly
	if !info.IsDir() {
		return r.deploySingleResource(ctx, path, maasPlatform, skipConfigMap)
	}

	// If it's a directory, check for kustomization.yaml
	kustomizationPath := filepath.Join(path, "kustomization.yaml")
	if _, err := os.Stat(kustomizationPath); err == nil {
		// This is a kustomize directory - we'll need to expand it
		// For now, let's read the kustomization and apply resources individually
		return r.deployKustomizeDirectory(ctx, path, maasPlatform, skipConfigMap)
	}

	// If no kustomization, apply all YAML files in directory
	return r.deployDirectoryYAMLs(ctx, path, maasPlatform, skipConfigMap)
}

// deployKustomizeDirectory handles kustomize directories
func (r *MaasPlatformReconciler) deployKustomizeDirectory(ctx context.Context, path string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap bool) error {
	// For now, we'll manually handle the resources listed in kustomization.yaml
	// In production, you might want to use kustomize library or exec
	log := logf.FromContext(ctx)
	_ = log

	kustomizationPath := filepath.Join(path, "kustomization.yaml")
	data, err := os.ReadFile(kustomizationPath)
	if err != nil {
		return fmt.Errorf("failed to read kustomization.yaml: %w", err)
	}

	// Parse kustomization to find resources
	var kustomization struct {
		Resources []string `yaml:"resources"`
	}
	if err := yaml.Unmarshal(data, &kustomization); err != nil {
		return fmt.Errorf("failed to parse kustomization.yaml: %w", err)
	}

	for _, resource := range kustomization.Resources {
		resourcePath := filepath.Join(path, resource)
		info, err := os.Stat(resourcePath)
		if err != nil {
			log.Info("Resource path does not exist, skipping", "path", resourcePath)
			continue
		}

		if info.IsDir() {
			// Recursive directory
			if err := r.deployResourcesFromPath(ctx, resourcePath, maasPlatform, skipConfigMap); err != nil {
				log.Error(err, "Failed to deploy resource", "path", resourcePath)
				// Continue with other resources
			}
		} else {
			// YAML file
			if err := r.deploySingleResource(ctx, resourcePath, maasPlatform, skipConfigMap); err != nil {
				log.Error(err, "Failed to deploy resource", "path", resourcePath)
				// Continue with other resources
			}
		}
	}

	return nil
}

// deployDirectoryYAMLs applies all YAML files in a directory
func (r *MaasPlatformReconciler) deployDirectoryYAMLs(ctx context.Context, dir string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if err := r.deploySingleResource(ctx, filePath, maasPlatform, skipConfigMap); err != nil {
			logf.FromContext(ctx).Error(err, "Failed to deploy YAML file", "file", filePath)
			// Continue with other files
		}
	}

	return nil
}

// deploySingleResource deploys a single YAML resource file
func (r *MaasPlatformReconciler) deploySingleResource(ctx context.Context, filePath string, maasPlatform *myappv1alpha1.MaasPlatform, skipConfigMap ...bool) error {
	log := logf.FromContext(ctx)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML (support multi-document YAML)
	documents, err := splitYAMLDocuments(data)
	if err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	skipCM := len(skipConfigMap) > 0 && skipConfigMap[0]

	for _, doc := range documents {
		if len(doc) == 0 {
			continue
		}

		var obj unstructured.Unstructured
		if err := yaml.Unmarshal(doc, &obj.Object); err != nil {
			log.Info("Skipping non-Kubernetes resource (failed to unmarshal)", "file", filePath)
			continue
		}

		// Skip ConfigMap if requested
		if skipCM && obj.GetKind() == "ConfigMap" && obj.GetName() == "tier-to-group-mapping" {
			log.Info("Skipping tier-to-group-mapping ConfigMap (managed by Tier controller)")
			continue
		}

		// Set owner reference
		if err := ctrl.SetControllerReference(maasPlatform, &obj, r.Scheme); err != nil {
			log.Info("Failed to set owner reference, continuing anyway", "error", err)
		}

		// Apply the resource
		if err := r.applyUnstructured(ctx, &obj); err != nil {
			return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}

		log.Info("Successfully deployed resource", "kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
	}

	return nil
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
