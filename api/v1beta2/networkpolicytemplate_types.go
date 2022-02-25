/*
Copyright 2021.

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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicyTemplateStatus defines the observed state of NetworkPolicyTemplate
//+kubebuilder:validation:Enum=ok;invalid
type NetworkPolicyTemplateStatus string

const (
	NetworkPolicyTemplateOK      NetworkPolicyTemplateStatus = "ok"
	NetworkPolicyTemplateInvalid NetworkPolicyTemplateStatus = "invalid"
)

// NetworkPolicyTemplateSpec defines the desired state of NetworkPolicyTemplate.
type NetworkPolicyTemplateSpec struct {
	// ClusterWide indicates whether the generated templates are clusterwide templates
	//+kubebuilder:default=false
	ClusterWide bool `json:"clusterwide,omitempty"`
	// PolicyTemplate is a template for creating NetworkPolicies
	PolicyTemplate string `json:"policyTemplate"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// NetworkPolicyTemplate is the Schema for the networkpolicytemplates API.
type NetworkPolicyTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the spec for the NetworkPolicyTemplate
	Spec NetworkPolicyTemplateSpec `json:"spec"`

	// Status represents the status of the NetworkPolicyTemplate
	// +optional
	Status NetworkPolicyTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkPolicyTemplateList contains a list of NetworkPolicyTemplate.
type NetworkPolicyTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkPolicyTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkPolicyTemplate{}, &NetworkPolicyTemplateList{})
}
