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
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	myappv1alpha1 "github.com/jland-redhat/maas-operator.git/api/v1alpha1"
)

// TierReconciler reconciles a Tier object
type TierReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=tiers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=tiers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=tiers/finalizers,verbs=update
// +kubebuilder:rbac:groups=myapp.io.odh.maas,resources=maasplatforms,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kuadrant.io,resources=ratelimitpolicies;tokenratelimitpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state specified by
// the Tier object.
func (r *TierReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Tier instance
	tier := &myappv1alpha1.Tier{}
	if err := r.Get(ctx, req.NamespacedName, tier); err != nil {
		if errors.IsNotFound(err) {
			// Tier was deleted, we need to reconcile all MaasPlatforms
			// since any of them could have been using this Tier
			log.Info("Tier was deleted, reconciling all MaasPlatforms")
			return r.reconcileAllMaasPlatforms(ctx)
		}
		log.Error(err, "Failed to get Tier")
		return ctrl.Result{}, err
	}

	// Get the target MaasPlatform
	maasPlatformNamespace := tier.Spec.TargetRef.Namespace
	if maasPlatformNamespace == "" {
		maasPlatformNamespace = tier.Namespace
	}

	maasPlatform := &myappv1alpha1.MaasPlatform{}
	maasPlatformKey := client.ObjectKey{
		Name:      tier.Spec.TargetRef.Name,
		Namespace: maasPlatformNamespace,
	}

	if err := r.Get(ctx, maasPlatformKey, maasPlatform); err != nil {
		log.Error(err, "Failed to get target MaasPlatform", "name", tier.Spec.TargetRef.Name, "namespace", maasPlatformNamespace)
		return ctrl.Result{}, err
	}

	// Reconcile all Tiers targeting this MaasPlatform (since we need to aggregate them)
	return r.reconcileMaasPlatformTiers(ctx, maasPlatform)
}

// reconcileAllMaasPlatforms reconciles all MaasPlatforms (used when Tier is deleted)
func (r *TierReconciler) reconcileAllMaasPlatforms(ctx context.Context) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	maasPlatformList := &myappv1alpha1.MaasPlatformList{}
	if err := r.List(ctx, maasPlatformList); err != nil {
		log.Error(err, "Failed to list MaasPlatforms")
		return ctrl.Result{}, err
	}

	for _, mp := range maasPlatformList.Items {
		if _, err := r.reconcileMaasPlatformTiers(ctx, &mp); err != nil {
			log.Error(err, "Failed to reconcile Tiers for MaasPlatform", "name", mp.Name)
			// Continue with other platforms
		}
	}

	return ctrl.Result{}, nil
}

// reconcileMaasPlatformTiers reconciles all Tiers targeting a specific MaasPlatform
func (r *TierReconciler) reconcileMaasPlatformTiers(ctx context.Context, maasPlatform *myappv1alpha1.MaasPlatform) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// List all Tiers
	tierList := &myappv1alpha1.TierList{}
	if err := r.List(ctx, tierList); err != nil {
		log.Error(err, "Failed to list Tiers")
		return ctrl.Result{}, err
	}

	// Filter Tiers targeting this MaasPlatform
	var targetTiers []myappv1alpha1.Tier
	for _, tier := range tierList.Items {
		tierNamespace := tier.Spec.TargetRef.Namespace
		if tierNamespace == "" {
			tierNamespace = tier.Namespace
		}

		if tier.Spec.TargetRef.Name == maasPlatform.Name && tierNamespace == maasPlatform.Namespace {
			targetTiers = append(targetTiers, tier)
		}
	}

	if len(targetTiers) == 0 {
		log.Info("No Tiers found targeting this MaasPlatform")
		return ctrl.Result{}, nil
	}

	// Update ConfigMap with tier mappings
	if err := r.updateTierConfigMap(ctx, targetTiers, maasPlatform); err != nil {
		log.Error(err, "Failed to update tier ConfigMap")
		return ctrl.Result{}, err
	}

	// Update RateLimitPolicy
	if err := r.updateRateLimitPolicy(ctx, targetTiers, maasPlatform); err != nil {
		log.Error(err, "Failed to update RateLimitPolicy")
		return ctrl.Result{}, err
	}

	// Update TokenRateLimitPolicy
	if err := r.updateTokenRateLimitPolicy(ctx, targetTiers, maasPlatform); err != nil {
		log.Error(err, "Failed to update TokenRateLimitPolicy")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateTierConfigMap updates the tier-to-group-mapping ConfigMap
func (r *TierReconciler) updateTierConfigMap(ctx context.Context, tiers []myappv1alpha1.Tier, maasPlatform *myappv1alpha1.MaasPlatform) error {
	log := logf.FromContext(ctx)

	configMapName := "tier-to-group-mapping"
	configMapNamespace := "maas-api" // Default namespace for maas-api

	configMap := &corev1.ConfigMap{}
	key := client.ObjectKey{
		Name:      configMapName,
		Namespace: configMapNamespace,
	}

	err := r.Get(ctx, key, configMap)
	if errors.IsNotFound(err) {
		// Create new ConfigMap
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: configMapNamespace,
			},
		}
	} else if err != nil {
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Build tier mapping YAML
	var tierMappings strings.Builder
	tierMappings.WriteString("tiers: |\n")

	// Sort tiers by name for consistent output
	sortedTiers := make([]myappv1alpha1.Tier, len(tiers))
	copy(sortedTiers, tiers)
	sort.Slice(sortedTiers, func(i, j int) bool {
		return sortedTiers[i].Name < sortedTiers[j].Name
	})

	for _, tier := range sortedTiers {
		tierName := tier.Name
		if tierName == "" {
			tierName = tier.GetGenerateName() + "-tier"
		}

		// Extract level from tier name (simplified - could be enhanced)
		level := 0
		if strings.Contains(strings.ToLower(tierName), "premium") {
			level = 1
		} else if strings.Contains(strings.ToLower(tierName), "enterprise") {
			level = 2
		}

		tierMappings.WriteString(fmt.Sprintf("  # Tier: %s\n", tierName))
		tierMappings.WriteString(fmt.Sprintf("  - name: %s\n", tierName))
		tierMappings.WriteString(fmt.Sprintf("    level: %d\n", level))
		tierMappings.WriteString("    groups:\n")
		// Use tier name as group (simplified - in production this should map to actual user groups)
		tierMappings.WriteString(fmt.Sprintf("      - tier-%s-users\n", tierName))
		tierMappings.WriteString("      - system:authenticated\n")
		tierMappings.WriteString("\n")
	}

	configMap.Data = map[string]string{
		"tiers": tierMappings.String(),
	}

	// Set owner reference to the first MaasPlatform
	if len(tiers) > 0 {
		// We can't set owner ref to multiple resources, so we'll use labels instead
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		configMap.Labels["app.kubernetes.io/managed-by"] = "maas-operator"
		configMap.Labels["maas-platform"] = fmt.Sprintf("%s.%s", maasPlatform.Name, maasPlatform.Namespace)
	}

	if err := r.Get(ctx, key, configMap); errors.IsNotFound(err) {
		if err := r.Create(ctx, configMap); err != nil {
			return fmt.Errorf("failed to create ConfigMap: %w", err)
		}
		log.Info("Created tier ConfigMap", "name", configMapName, "namespace", configMapNamespace)
	} else {
		if err := r.Update(ctx, configMap); err != nil {
			return fmt.Errorf("failed to update ConfigMap: %w", err)
		}
		log.Info("Updated tier ConfigMap", "name", configMapName, "namespace", configMapNamespace)
	}

	return nil
}

// updateRateLimitPolicy updates or creates the RateLimitPolicy
func (r *TierReconciler) updateRateLimitPolicy(ctx context.Context, tiers []myappv1alpha1.Tier, maasPlatform *myappv1alpha1.MaasPlatform) error {
	log := logf.FromContext(ctx)

	policyName := "gateway-rate-limits"
	policyNamespace := "openshift-ingress"

	policy := &unstructured.Unstructured{}
	policy.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "kuadrant.io",
		Version: "v1",
		Kind:    "RateLimitPolicy",
	})

	key := client.ObjectKey{
		Name:      policyName,
		Namespace: policyNamespace,
	}

	err := r.Get(ctx, key, policy)
	if errors.IsNotFound(err) {
		// Create new policy
		policy.SetName(policyName)
		policy.SetNamespace(policyNamespace)
	} else if err != nil {
		return fmt.Errorf("failed to get RateLimitPolicy: %w", err)
	}

	// Build limits from tiers
	limits := make(map[string]interface{})
	for _, tier := range tiers {
		if tier.Spec.RateLimits == nil {
			continue
		}

		tierName := tier.Name
		if tierName == "" {
			tierName = tier.GetGenerateName() + "-tier"
		}

		tierLimit := map[string]interface{}{
			"rates": []map[string]interface{}{
				{
					"limit":  tier.Spec.RateLimits.Limit,
					"window": tier.Spec.RateLimits.Window,
				},
			},
			"when": []map[string]interface{}{
				{
					"predicate": fmt.Sprintf(`auth.identity.tier == "%s"`, tierName),
				},
			},
		}

		counters := tier.Spec.RateLimits.Counters
		if len(counters) == 0 {
			counters = []string{"auth.identity.userid"}
		}
		tierLimit["counters"] = counters

		limits[tierName] = tierLimit
	}

	// Set spec
	if err := unstructured.SetNestedMap(policy.Object, map[string]interface{}{
		"targetRef": map[string]interface{}{
			"group": "gateway.networking.k8s.io",
			"kind":  "Gateway",
			"name":  "maas-default-gateway",
		},
		"limits": limits,
	}, "spec"); err != nil {
		return fmt.Errorf("failed to set policy spec: %w", err)
	}

	if err := r.Get(ctx, key, policy); errors.IsNotFound(err) {
		if err := r.Create(ctx, policy); err != nil {
			return fmt.Errorf("failed to create RateLimitPolicy: %w", err)
		}
		log.Info("Created RateLimitPolicy", "name", policyName)
	} else {
		if err := r.Update(ctx, policy); err != nil {
			return fmt.Errorf("failed to update RateLimitPolicy: %w", err)
		}
		log.Info("Updated RateLimitPolicy", "name", policyName)
	}

	return nil
}

// updateTokenRateLimitPolicy updates or creates the TokenRateLimitPolicy
func (r *TierReconciler) updateTokenRateLimitPolicy(ctx context.Context, tiers []myappv1alpha1.Tier, maasPlatform *myappv1alpha1.MaasPlatform) error {
	log := logf.FromContext(ctx)

	policyName := "gateway-token-rate-limits"
	policyNamespace := "openshift-ingress"

	policy := &unstructured.Unstructured{}
	policy.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "kuadrant.io",
		Version: "v1alpha1",
		Kind:    "TokenRateLimitPolicy",
	})

	key := client.ObjectKey{
		Name:      policyName,
		Namespace: policyNamespace,
	}

	err := r.Get(ctx, key, policy)
	if errors.IsNotFound(err) {
		// Create new policy
		policy.SetName(policyName)
		policy.SetNamespace(policyNamespace)
	} else if err != nil {
		return fmt.Errorf("failed to get TokenRateLimitPolicy: %w", err)
	}

	// Build limits from tiers
	limits := make(map[string]interface{})
	for _, tier := range tiers {
		if tier.Spec.TokenRateLimits == nil {
			continue
		}

		tierName := tier.Name
		if tierName == "" {
			tierName = tier.GetGenerateName() + "-tier"
		}

		limitName := fmt.Sprintf("%s-user-tokens", tierName)
		tierLimit := map[string]interface{}{
			"rates": []map[string]interface{}{
				{
					"limit":  tier.Spec.TokenRateLimits.Limit,
					"window": tier.Spec.TokenRateLimits.Window,
				},
			},
			"when": []map[string]interface{}{
				{
					"predicate": fmt.Sprintf(`auth.identity.tier == "%s"`, tierName),
				},
			},
		}

		counters := tier.Spec.TokenRateLimits.Counters
		if len(counters) == 0 {
			counters = []string{"auth.identity.userid"}
		}
		tierLimit["counters"] = counters

		limits[limitName] = tierLimit
	}

	// Set spec
	if err := unstructured.SetNestedMap(policy.Object, map[string]interface{}{
		"targetRef": map[string]interface{}{
			"group": "gateway.networking.k8s.io",
			"kind":  "Gateway",
			"name":  "maas-default-gateway",
		},
		"limits": limits,
	}, "spec"); err != nil {
		return fmt.Errorf("failed to set policy spec: %w", err)
	}

	if err := r.Get(ctx, key, policy); errors.IsNotFound(err) {
		if err := r.Create(ctx, policy); err != nil {
			return fmt.Errorf("failed to create TokenRateLimitPolicy: %w", err)
		}
		log.Info("Created TokenRateLimitPolicy", "name", policyName)
	} else {
		if err := r.Update(ctx, policy); err != nil {
			return fmt.Errorf("failed to update TokenRateLimitPolicy: %w", err)
		}
		log.Info("Updated TokenRateLimitPolicy", "name", policyName)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TierReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&myappv1alpha1.Tier{}).
		Named("tier").
		Complete(r)
}
