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

// TierSpec defines the desired state of Tier.
type TierSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// TargetRef references the MaasPlatform this tier applies to
	TargetRef MaasPlatformTargetRef `json:"targetRef"`

	// Rate limits for requests (HTTP requests per time window)
	// +optional
	RateLimits *TierRateLimitConfig `json:"rateLimits,omitempty"`

	// Token rate limits (tokens per time window from model responses)
	// +optional
	TokenRateLimits *TierTokenRateLimitConfig `json:"tokenRateLimits,omitempty"`

	// Models that this tier applies to
	// If empty, applies to all models. If specified, only these models are affected.
	// +optional
	Models []string `json:"models,omitempty"`
}

// MaasPlatformTargetRef references a MaasPlatform resource
type MaasPlatformTargetRef struct {
	// Name of the MaasPlatform resource
	Name string `json:"name"`

	// Namespace of the MaasPlatform resource
	// If empty, defaults to the same namespace as the Tier resource
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TierRateLimitConfig defines rate limit configuration for requests.
type TierRateLimitConfig struct {
	// Maximum number of requests allowed
	Limit int32 `json:"limit"`

	// Time window for the rate limit (e.g., "2m", "1h", "30s")
	Window string `json:"window"`

	// Counter expressions for rate limit tracking
	// Default: ["auth.identity.userid"]
	// +optional
	Counters []string `json:"counters,omitempty"`
}

// TierTokenRateLimitConfig defines token rate limit configuration.
type TierTokenRateLimitConfig struct {
	// Maximum number of tokens allowed
	Limit int32 `json:"limit"`

	// Time window for the token rate limit (e.g., "1m", "1h", "30s")
	Window string `json:"window"`

	// Counter expressions for token rate limit tracking
	// Default: ["auth.identity.userid"]
	// +optional
	Counters []string `json:"counters,omitempty"`
}

// TierStatus defines the observed state of Tier.
type TierStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Tier is the Schema for the tiers API.
type Tier struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TierSpec   `json:"spec,omitempty"`
	Status TierStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TierList contains a list of Tier.
type TierList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tier `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tier{}, &TierList{})
}
