package cilium

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

//revive:disable:exported
func CiliumClusterwideNetworkPolicy() *unstructured.Unstructured {
	ccnp := &unstructured.Unstructured{}
	ccnp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: CiliumNetworkPolicyVersion,
		Kind:    "CiliumClusterwideNetworkPolicy",
	})
	return ccnp
}

func CiliumClusterwideNetworkPolicyList() *unstructured.UnstructuredList {
	ccnpl := &unstructured.UnstructuredList{}
	ccnpl.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: CiliumNetworkPolicyVersion,
		Kind:    "CiliumClusterwideNetworkPolicyList",
	})
	return ccnpl
}
