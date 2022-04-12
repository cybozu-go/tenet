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

// NetworkPolicyAdmissionRuleStatus defines the observed state of NetworkPolicyAdmissionRule.
type NetworkPolicyAdmissionRuleStatus string

// NetworkPolicyAdmissionRuleType defines the type of network connection the rules apply to.
type NetworkPolicyAdmissionRuleType string

const (
	NetworkPolicyAdmissionRuleOK NetworkPolicyAdmissionRuleStatus = "ok"

	NetworkPolicyAdmissionRuleTypeAll     NetworkPolicyAdmissionRuleType = "all"
	NetworkPolicyAdmissionRuleTypeEgress  NetworkPolicyAdmissionRuleType = "egress"
	NetworkPolicyAdmissionRuleTypeIngress NetworkPolicyAdmissionRuleType = "ingress"
)

// NetworkPolicyAdmissionRuleSpec defines the desired state of NetworkPolicyAdmissionRule.
type NetworkPolicyAdmissionRuleSpec struct {
	// NamespaceSelector qualifies which namespaces the rules should apply to.
	NamespaceSelector NetworkPolicyAdmissionRuleNamespaceSelector `json:"namespaceSelector,omitempty"`
	// ForbiddenIPRanges defines IP ranges whose usage must be forbidden in network policies.
	ForbiddenIPRanges []NetworkPolicyAdmissionRuleForbiddenIPRanges `json:"forbiddenIPRanges,omitempty"`
	// ForbiddenEntities defines entities whose usage must be forbidden in network policies.
	ForbiddenEntities []NetworkPolicyAdmissionRuleForbiddenEntity `json:"forbiddenEntities,omitempty"`
}

// NetworkPolicyAdmissionRuleNamespaceSelector defines how namespaces should be selected.
type NetworkPolicyAdmissionRuleNamespaceSelector struct {
	// ExcludeLabels defines labels through which a namespace should be excluded.
	ExcludeLabels map[string]string `json:"excludeLabels,omitempty"`
}

// NetworkPolicyAdmissionRuleForbiddenIPRanges defines forbidden IP ranges.
type NetworkPolicyAdmissionRuleForbiddenIPRanges struct {
	// CIDR range.
	CIDR string `json:"cidr"`

	// Type of connection the rule applies to.
	// +kubebuilder:validation:Enum=egress;ingress;all
	// +default:"all"
	Type NetworkPolicyAdmissionRuleType `json:"type"`
}

// NetworkPolicyAdmissionRuleForbiddenEntity defines forbidden entities.
type NetworkPolicyAdmissionRuleForbiddenEntity struct {
	// Entity name.
	Entity string `json:"entity"`

	// Type of connection the rule applies to.
	// +kubebuilder:validation:Enum=egress;ingress;all
	// +default:"all"
	Type NetworkPolicyAdmissionRuleType `json:"type"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// NetworkPolicyAdmissionRule is the Schema for the networkpolicyadmissionrules API.
type NetworkPolicyAdmissionRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkPolicyAdmissionRuleSpec   `json:"spec"`
	Status NetworkPolicyAdmissionRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkPolicyAdmissionRuleList contains a list of NetworkPolicyAdmissionRule.
type NetworkPolicyAdmissionRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkPolicyAdmissionRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkPolicyAdmissionRule{}, &NetworkPolicyAdmissionRuleList{})
}
