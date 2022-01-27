package cilium

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

//revive:disable:exported
func CiliumNetworkPolicy() *unstructured.Unstructured {
	cnp := &unstructured.Unstructured{}
	cnp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: CiliumNetworkPolicyVersion,
		Kind:    "CiliumNetworkPolicy",
	})
	return cnp
}

func CiliumNetworkPolicyList() *unstructured.UnstructuredList {
	cnpl := &unstructured.UnstructuredList{}
	cnpl.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: CiliumNetworkPolicyVersion,
		Kind:    "CiliumNetworkPolicyList",
	})
	return cnpl
}
