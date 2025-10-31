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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PoliciesConfig defines the configuration for gateway policies including authentication,
// rate limiting, and token rate limiting.
type PoliciesConfig struct {
	// Authentication policy configuration
	// +optional
	Authentication *AuthenticationConfig `json:"authentication,omitempty"`

	// Rate limiting policy configuration
	// +optional
	RateLimits *RateLimitConfig `json:"rateLimits,omitempty"`

	// Token rate limiting policy configuration
	// +optional
	TokenRateLimits *TokenRateLimitConfig `json:"tokenRateLimits,omitempty"`
}

// AuthenticationConfig defines the authentication policy configuration.
type AuthenticationConfig struct {
	// Enable authentication policy
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// OpenShift identities configuration for token validation
	// +optional
	OpenShiftIdentities *OpenShiftAuthConfig `json:"openshiftIdentities,omitempty"`

	// Tier metadata lookup configuration for enriching identity with subscription tier
	// +optional
	TierMetadata *TierMetadataConfig `json:"tierMetadata,omitempty"`

	// Authorization rules for tier-based access control
	// +optional
	Authorization *AuthorizationConfig `json:"authorization,omitempty"`

	// Response filters for enriching response with identity information
	// +optional
	ResponseFilters *ResponseFiltersConfig `json:"responseFilters,omitempty"`
}

// OpenShiftAuthConfig defines OpenShift identity authentication configuration.
type OpenShiftAuthConfig struct {
	// Kubernetes token review audiences
	// +optional
	Audiences []string `json:"audiences,omitempty"`

	// User ID extraction expression for token normalization
	// Default: extracts the last component of system:serviceaccount:<ns>:<name>
	// +optional
	UserIDExpression string `json:"userIDExpression,omitempty"`
}

// TierMetadataConfig defines the tier metadata lookup configuration.
type TierMetadataConfig struct {
	// API endpoint URL for tier lookup service
	// Default: http://maas-api.maas-api.svc.cluster.local:8080/v1/tiers/lookup
	// +optional
	APIEndpoint string `json:"apiEndpoint,omitempty"`

	// Cache TTL in seconds for tier metadata
	// Default: 300 (5 minutes)
	// +kubebuilder:default=300
	CacheTTL int32 `json:"cacheTTL,omitempty"`

	// Cache key selector for tier metadata caching
	// Default: auth.identity.user.username
	// +optional
	CacheKeySelector string `json:"cacheKeySelector,omitempty"`
}

// AuthorizationConfig defines authorization rules for resource access.
type AuthorizationConfig struct {
	// Enable tier-based authorization
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Resource attributes for Kubernetes Subject Access Review
	// +optional
	ResourceAttributes *ResourceAttributesConfig `json:"resourceAttributes,omitempty"`
}

// ResourceAttributesConfig defines resource attributes for authorization checks.
type ResourceAttributesConfig struct {
	// API group for the resource
	// Default: serving.kserve.io
	// +optional
	Group string `json:"group,omitempty"`

	// Resource type
	// Default: llminferenceservices
	// +optional
	Resource string `json:"resource,omitempty"`

	// Verb for the resource access
	// Default: post
	// +optional
	Verb string `json:"verb,omitempty"`

	// Namespace extraction expression from request path
	// Default: request.path.split("/")[1]
	// +optional
	NamespaceExpression string `json:"namespaceExpression,omitempty"`

	// Name extraction expression from request path
	// Default: request.path.split("/")[2]
	// +optional
	NameExpression string `json:"nameExpression,omitempty"`
}

// ResponseFiltersConfig defines response filters for enriching responses.
type ResponseFiltersConfig struct {
	// Enable response filters
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Include user ID in response
	// +optional
	IncludeUserID bool `json:"includeUserID,omitempty"`

	// Include tier information in response
	// +optional
	IncludeTier bool `json:"includeTier,omitempty"`
}

// RateLimitConfig defines the rate limiting policy configuration.
type RateLimitConfig struct {
	// Enable rate limiting policy
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Tier-based rate limits
	// Key is the tier name (e.g., "free", "premium", "enterprise")
	// +optional
	Tiers map[string]TierRateLimit `json:"tiers,omitempty"`
}

// TierRateLimit defines rate limit configuration for a specific tier.
type TierRateLimit struct {
	// Maximum number of requests allowed
	Limit int32 `json:"limit"`

	// Time window for the rate limit (e.g., "2m", "1h")
	Window string `json:"window"`

	// Counter expressions for rate limit tracking
	// Default: ["auth.identity.userid"]
	// +optional
	Counters []string `json:"counters,omitempty"`
}

// TokenRateLimitConfig defines the token-based rate limiting policy configuration.
type TokenRateLimitConfig struct {
	// Enable token rate limiting policy
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Tier-based token rate limits
	// Key is the tier name (e.g., "free", "premium", "enterprise")
	// +optional
	Tiers map[string]TierTokenLimit `json:"tiers,omitempty"`
}

// TierTokenLimit defines token rate limit configuration for a specific tier.
type TierTokenLimit struct {
	// Maximum number of tokens allowed
	Limit int32 `json:"limit"`

	// Time window for the token rate limit (e.g., "1m", "1h")
	Window string `json:"window"`

	// Counter expressions for token rate limit tracking
	// Default: ["auth.identity.userid"]
	// +optional
	Counters []string `json:"counters,omitempty"`
}

// MaasPlatformSpec defines the desired state of MaasPlatform.
type MaasPlatformSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Policies configuration for authentication, rate limiting, and token rate limiting
	// +optional
	Policies *PoliciesConfig `json:"policies,omitempty"`
}

// MaasPlatformStatus defines the observed state of MaasPlatform.
type MaasPlatformStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MaasPlatform is the Schema for the maasplatforms API.
type MaasPlatform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaasPlatformSpec   `json:"spec,omitempty"`
	Status MaasPlatformStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MaasPlatformList contains a list of MaasPlatform.
type MaasPlatformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaasPlatform `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaasPlatform{}, &MaasPlatformList{})
}
