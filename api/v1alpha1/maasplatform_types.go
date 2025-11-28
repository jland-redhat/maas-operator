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
// Policy configuration (rate limits, token limits) has been moved to Tier resources.

// MaasPlatformSpec defines the desired state of MaasPlatform.
type MaasPlatformSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Policies are now configured via Tier resources that reference this MaasPlatform
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
