package hooks

import (
	"context"
	"fmt"
	"net"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tenetv1beta1 "github.com/cybozu-go/tenet/api/v1beta1"
	"github.com/cybozu-go/tenet/pkg/cilium"
)

//+kubebuilder:webhook:path=/validate-cilium-io-v2-ciliumnetworkpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=cilium.io,resources=ciliumnetworkpolicies,verbs=create;update;delete,versions=v2,name=vciliumnetworkpolicy.kb.io,admissionReviewVersions={v1}

type ciliumNetworkPolicyValidator struct {
	client.Client
	dec *admission.Decoder
}

var _ admission.Handler = &ciliumNetworkPolicyValidator{}

// Handler validates CiliumNetworkPolicies.
func (v *ciliumNetworkPolicyValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.Operation {
	case admissionv1.Delete:
		return v.handleDelete(ctx, req)
	case admissionv1.Create:
		return v.handleCreateOrUpdate(ctx, req)
	case admissionv1.Update:
		return v.handleCreateOrUpdate(ctx, req)
	default:
		return admission.Allowed("")
	}
}

func (v *ciliumNetworkPolicyValidator) handleDelete(_ context.Context, req admission.Request) admission.Response {
	cnp := cilium.CiliumNetworkPolicy()
	if err := v.dec.DecodeRaw(req.OldObject, cnp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	owners := cnp.GetOwnerReferences()
	for _, owner := range owners {
		if owner.APIVersion == tenetv1beta1.GroupVersion.String() && owner.Kind == tenetv1beta1.NetworkPolicyTemplateKind {
			for _, g := range req.UserInfo.Groups {
				if g == "system:serviceaccounts" {
					return admission.Allowed("")
				}
			}
			return admission.Denied("user deletion is not allowed")
		}
	}
	return admission.Allowed("")
}

func (v *ciliumNetworkPolicyValidator) handleCreateOrUpdate(ctx context.Context, req admission.Request) admission.Response {
	cnp := cilium.CiliumNetworkPolicy()
	if err := v.dec.Decode(req, cnp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	ns := &corev1.Namespace{}
	if err := v.Get(ctx, client.ObjectKey{Name: cnp.GetNamespace()}, ns); client.IgnoreNotFound(err) != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	var nparl tenetv1beta1.NetworkPolicyAdmissionRuleList
	if err := v.List(ctx, &nparl); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if !v.shouldValidate(ns, &nparl) {
		return admission.Allowed("")
	}

	egressPolicies, ingressPolicies, err := v.gatherPolicies(cnp)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	egressFilters, ingressFilters, err := v.gatherFilters(&nparl)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return v.validate(egressPolicies, ingressPolicies, egressFilters, ingressFilters)
}

func (v *ciliumNetworkPolicyValidator) gatherPolicies(cnp *unstructured.Unstructured) ([]*net.IPNet, []*net.IPNet, error) {
	var egressPolicies, ingressPolicies []*net.IPNet
	cnpSpec, found, _ := unstructured.NestedMap(cnp.UnstructuredContent(), "spec")
	if found {
		e, i, err := v.gatherPoliciesFromRule(cnpSpec)
		if err != nil {
			return nil, nil, err
		}
		egressPolicies = append(egressPolicies, e...)
		ingressPolicies = append(ingressPolicies, i...)
	}
	cnpSpecs, found, _ := unstructured.NestedSlice(cnp.UnstructuredContent(), "specs")
	if found {
		for _, cnpSpec := range cnpSpecs {
			rule, ok := cnpSpec.(map[string]interface{})
			if !ok {
				return nil, nil, fmt.Errorf("unexpected spec format")
			}
			e, i, err := v.gatherPoliciesFromRule(rule)
			if err != nil {
				return nil, nil, err
			}
			egressPolicies = append(egressPolicies, e...)
			ingressPolicies = append(ingressPolicies, i...)
		}
	}
	return egressPolicies, ingressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromRule(rule map[string]interface{}) ([]*net.IPNet, []*net.IPNet, error) {
	egressPolicies, err := v.gatherPoliciesFromRuleType(rule, cilium.EgressRule)
	if err != nil {
		return nil, nil, err
	}
	ingressPolicies, err := v.gatherPoliciesFromRuleType(rule, cilium.IngressRule)
	if err != nil {
		return nil, nil, err
	}
	return egressPolicies, ingressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromRuleType(rule map[string]interface{}, rt cilium.RuleType) ([]*net.IPNet, error) {
	subRule := rule[rt.Type]
	if subRule == nil {
		return nil, nil
	}
	policies := []*net.IPNet{}
	subRules, ok := subRule.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected policies format")
	}
	for _, r := range subRules {
		rMap, ok := r.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected policy format")
		}
		p, err := v.gatherPoliciesFromCIDRRule(rMap[rt.CIDRKey])
		if err != nil {
			return nil, err
		}
		policies = append(policies, p...)

		p, err = v.gatherPoliciesFromCIDRSetRule(rMap[rt.CIDRSetKey])
		if err != nil {
			return nil, err
		}
		policies = append(policies, p...)
	}
	return policies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromCIDRRule(rule interface{}) ([]*net.IPNet, error) {
	if rule == nil {
		return nil, nil
	}
	policies := []*net.IPNet{}
	cidrStrings, ok := rule.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected CIDR strings format")
	}
	for _, cidrString := range cidrStrings {
		if cidrString == nil {
			continue
		}
		cidrString, ok := cidrString.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected CIDR string format")
		}
		_, cidr, err := net.ParseCIDR(cidrString)
		if err != nil {
			return nil, err
		}
		policies = append(policies, cidr)
	}
	return policies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromCIDRSetRule(rule interface{}) ([]*net.IPNet, error) {
	if rule == nil {
		return nil, nil
	}
	cidrSetRules, ok := rule.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected CIDRSet policies format")
	}
	var policies []*net.IPNet
	for _, cidrSetRule := range cidrSetRules {
		cidrSetRule, ok := cidrSetRule.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected CIDRSet format")
		}
		if cidrSetRule["cidr"] == nil {
			continue
		}
		cidrString, ok := cidrSetRule["cidr"].(string)
		if !ok {
			return nil, fmt.Errorf("unexpected CIDR string format")
		}
		_, cidr, err := net.ParseCIDR(cidrString)
		if err != nil {
			return nil, err
		}
		policies = append(policies, cidr)
	}
	return policies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherFilters(nparl *tenetv1beta1.NetworkPolicyAdmissionRuleList) ([]*net.IPNet, []*net.IPNet, error) {
	var egressFilters, ingressFilters []*net.IPNet
	for _, npar := range nparl.Items {
		for _, ipRange := range npar.Spec.ForbiddenIPRanges {
			_, cidr, err := net.ParseCIDR(ipRange.CIDR)
			if err != nil {
				return nil, nil, err
			}
			switch ipRange.Type {
			case tenetv1beta1.NetworkPolicyAdmissionRuleTypeAll:
				egressFilters = append(egressFilters, cidr)
				ingressFilters = append(ingressFilters, cidr)
			case tenetv1beta1.NetworkPolicyAdmissionRuleTypeEgress:
				egressFilters = append(egressFilters, cidr)
			case tenetv1beta1.NetworkPolicyAdmissionRuleTypeIngress:
				ingressFilters = append(ingressFilters, cidr)
			}
		}
	}
	return egressFilters, ingressFilters, nil
}

func (v *ciliumNetworkPolicyValidator) intersect(cidr1, cidr2 *net.IPNet) bool {
	return cidr1.Contains(cidr2.IP) || cidr2.Contains(cidr1.IP)
}

func (v *ciliumNetworkPolicyValidator) validate(egressPolicies, ingressPolicies, egressFilters, ingressFilters []*net.IPNet) admission.Response {
	for _, egressPolicy := range egressPolicies {
		for _, egressFilter := range egressFilters {
			if v.intersect(egressPolicy, egressFilter) {
				return admission.Denied("an egress policy is requesting a forbidden IP range")
			}
		}
	}
	for _, ingressPolicy := range ingressPolicies {
		for _, ingressFilter := range ingressFilters {
			if v.intersect(ingressPolicy, ingressFilter) {
				return admission.Denied("an ingress policy is requesting a forbidden IP range")
			}
		}
	}
	return admission.Allowed("")
}

func (v *ciliumNetworkPolicyValidator) shouldValidate(ns *corev1.Namespace, nparl *tenetv1beta1.NetworkPolicyAdmissionRuleList) bool {
	for _, npar := range nparl.Items {
		for k, v := range npar.Spec.NamespaceSelector.ExcludeLabels {
			if ns.Labels[k] == v {
				return false
			}
		}
	}
	return true
}

func SetupCiliumNetworkPolicyWebhook(mgr manager.Manager, dec *admission.Decoder) {
	v := &ciliumNetworkPolicyValidator{
		Client: mgr.GetClient(),
		dec:    dec,
	}
	srv := mgr.GetWebhookServer()
	srv.Register("/validate-cilium-io-v2-ciliumnetworkpolicy", &webhook.Admission{Handler: v})
}
