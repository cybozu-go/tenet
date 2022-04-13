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

	tenetv1beta2 "github.com/cybozu-go/tenet/api/v1beta2"
	"github.com/cybozu-go/tenet/pkg/cilium"
)

//+kubebuilder:webhook:path=/validate-cilium-io-v2-ciliumnetworkpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=cilium.io,resources=ciliumnetworkpolicies,verbs=create;update;delete,versions=v2,name=vciliumnetworkpolicy.kb.io,admissionReviewVersions={v1}

type ciliumNetworkPolicyValidator struct {
	client.Client
	dec                *admission.Decoder
	serviceAccountName string
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
		if owner.APIVersion == tenetv1beta2.GroupVersion.String() && owner.Kind == tenetv1beta2.NetworkPolicyTemplateKind {
			if req.UserInfo.Username == v.serviceAccountName {
				return admission.Allowed("deletion by service account")
			}
			return admission.Denied("user deletion is not allowed")
		}
	}
	return admission.Allowed("")
}

func (v *ciliumNetworkPolicyValidator) handleCreateOrUpdate(ctx context.Context, req admission.Request) admission.Response {
	var res admission.Response

	cnp := cilium.CiliumNetworkPolicy()
	if err := v.dec.Decode(req, cnp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	ns := &corev1.Namespace{}
	if err := v.Get(ctx, client.ObjectKey{Name: cnp.GetNamespace()}, ns); client.IgnoreNotFound(err) != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	var nparl tenetv1beta2.NetworkPolicyAdmissionRuleList
	if err := v.List(ctx, &nparl); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if !v.shouldValidate(ns, &nparl) {
		return admission.Allowed("")
	}

	res = v.validateIP(nparl, cnp)
	if !res.Allowed {
		return res
	}

	return res
}

func (v *ciliumNetworkPolicyValidator) gatherIPPolicies(cnp *unstructured.Unstructured) ([]*net.IPNet, []*net.IPNet, error) {
	var egressPolicies, ingressPolicies []*net.IPNet
	e, i, err := v.gatherPolicies(cnp, cilium.CIDRRuleKey, v.gatherPoliciesFromStringRule)
	if err != nil {
		return nil, nil, err
	}
	es, err := v.toIPNetSlice(e)
	if err != nil {
		return nil, nil, err
	}
	egressPolicies = append(egressPolicies, es...)
	is, err := v.toIPNetSlice(i)
	if err != nil {
		return nil, nil, err
	}
	ingressPolicies = append(ingressPolicies, is...)
	e, i, err = v.gatherPolicies(cnp, cilium.CIDRSetRuleKey, v.gatherPoliciesFromCIDRSetRule)
	if err != nil {
		return nil, nil, err
	}
	es, err = v.toIPNetSlice(e)
	if err != nil {
		return nil, nil, err
	}
	egressPolicies = append(egressPolicies, es...)
	is, err = v.toIPNetSlice(i)
	if err != nil {
		return nil, nil, err
	}
	ingressPolicies = append(ingressPolicies, is...)
	return egressPolicies, ingressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) toIPNetSlice(raw []string) ([]*net.IPNet, error) {
	var res []*net.IPNet
	for _, str := range raw {
		_, cidr, err := net.ParseCIDR(str)
		if err != nil {
			return nil, err
		}
		res = append(res, cidr)
	}
	return res, nil
}

func (v *ciliumNetworkPolicyValidator) gatherIPFilters(nparl *tenetv1beta2.NetworkPolicyAdmissionRuleList) ([]*net.IPNet, []*net.IPNet, error) {
	var egressFilters, ingressFilters []*net.IPNet
	for _, npar := range nparl.Items {
		for _, ipRange := range npar.Spec.ForbiddenIPRanges {
			_, cidr, err := net.ParseCIDR(ipRange.CIDR)
			if err != nil {
				return nil, nil, err
			}
			switch ipRange.Type {
			case tenetv1beta2.NetworkPolicyAdmissionRuleTypeAll:
				egressFilters = append(egressFilters, cidr)
				ingressFilters = append(ingressFilters, cidr)
			case tenetv1beta2.NetworkPolicyAdmissionRuleTypeEgress:
				egressFilters = append(egressFilters, cidr)
			case tenetv1beta2.NetworkPolicyAdmissionRuleTypeIngress:
				ingressFilters = append(ingressFilters, cidr)
			}
		}
	}
	return egressFilters, ingressFilters, nil
}

func (v *ciliumNetworkPolicyValidator) intersectIP(cidr1, cidr2 *net.IPNet) bool {
	return cidr1.Contains(cidr2.IP) || cidr2.Contains(cidr1.IP)
}

func (v *ciliumNetworkPolicyValidator) getRulesFromSpec(cnp *unstructured.Unstructured) ([]map[string]interface{}, error) {
	var rules []map[string]interface{}
	cnpSpec, found, _ := unstructured.NestedMap(cnp.UnstructuredContent(), "spec")
	if found {
		rules = append(rules, cnpSpec)
	}
	cnpSpecs, found, _ := unstructured.NestedSlice(cnp.UnstructuredContent(), "specs")
	if found {
		for _, cnpSpec := range cnpSpecs {
			rule, ok := cnpSpec.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("unexpected spec format")
			}
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPolicies(cnp *unstructured.Unstructured, ruleKey cilium.RuleKey, gatherFunc func(interface{}) ([]string, error)) ([]string, []string, error) {
	var egressPolicies, ingressPolicies []string
	rules, err := v.getRulesFromSpec(cnp)
	if err != nil {
		return nil, nil, err
	}
	for _, rule := range rules {
		e, i, err := v.gatherPoliciesFromRule(rule, ruleKey, gatherFunc)
		if err != nil {
			return nil, nil, err
		}
		egressPolicies = append(egressPolicies, e...)
		ingressPolicies = append(ingressPolicies, i...)
	}
	return egressPolicies, ingressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromRule(rule map[string]interface{}, ruleKey cilium.RuleKey, gatherFunc func(interface{}) ([]string, error)) ([]string, []string, error) {
	egressPolicies, err := v.gatherPoliciesFromRuleType(rule, cilium.EgressRule, ruleKey, gatherFunc)
	if err != nil {
		return nil, nil, err
	}
	ingressPolicies, err := v.gatherPoliciesFromRuleType(rule, cilium.IngressRule, ruleKey, gatherFunc)
	if err != nil {
		return nil, nil, err
	}
	return egressPolicies, ingressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromRuleType(rule map[string]interface{}, ruleType cilium.RuleType, ruleKey cilium.RuleKey, gatherFunc func(interface{}) ([]string, error)) ([]string, error) {
	var policies []string
	subRules, found, err := unstructured.NestedSlice(rule, ruleType.Type)
	if !found {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, r := range subRules {
		rMap, ok := r.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected policy format")
		}
		p, err := gatherFunc(rMap[ruleType.RuleKeys[ruleKey]])
		if err != nil {
			return nil, err
		}
		policies = append(policies, p...)
	}
	return policies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromStringRule(rule interface{}) ([]string, error) {
	if rule == nil {
		return nil, nil
	}
	var policies []string
	stringRules, ok := rule.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected entity strings format")
	}
	for _, stringRule := range stringRules {
		if stringRule == nil {
			continue
		}
		str, ok := stringRule.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected entity string format")
		}
		policies = append(policies, str)
	}
	return policies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromCIDRSetRule(rule interface{}) ([]string, error) {
	if rule == nil {
		return nil, nil
	}
	cidrSetRules, ok := rule.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected CIDRSet policies format")
	}
	var policies []string
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
		policies = append(policies, cidrString)
	}
	return policies, nil
}

func (v *ciliumNetworkPolicyValidator) validateIP(nparl tenetv1beta2.NetworkPolicyAdmissionRuleList, cnp *unstructured.Unstructured) admission.Response {
	egressPolicies, ingressPolicies, err := v.gatherIPPolicies(cnp)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	egressFilters, ingressFilters, err := v.gatherIPFilters(&nparl)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	for _, egressPolicy := range egressPolicies {
		for _, egressFilter := range egressFilters {
			if v.intersectIP(egressPolicy, egressFilter) {
				return admission.Denied("an egress policy is requesting a forbidden IP range")
			}
		}
	}
	for _, ingressPolicy := range ingressPolicies {
		for _, ingressFilter := range ingressFilters {
			if v.intersectIP(ingressPolicy, ingressFilter) {
				return admission.Denied("an ingress policy is requesting a forbidden IP range")
			}
		}
	}
	return admission.Allowed("")
}

func (v *ciliumNetworkPolicyValidator) shouldValidate(ns *corev1.Namespace, nparl *tenetv1beta2.NetworkPolicyAdmissionRuleList) bool {
	for _, npar := range nparl.Items {
		for k, v := range npar.Spec.NamespaceSelector.ExcludeLabels {
			if ns.Labels[k] == v {
				return false
			}
		}
	}
	return true
}

func SetupCiliumNetworkPolicyWebhook(mgr manager.Manager, dec *admission.Decoder, sa string) {
	v := &ciliumNetworkPolicyValidator{
		Client:             mgr.GetClient(),
		dec:                dec,
		serviceAccountName: sa,
	}
	srv := mgr.GetWebhookServer()
	srv.Register("/validate-cilium-io-v2-ciliumnetworkpolicy", &webhook.Admission{Handler: v})
}
