package hooks

import (
	"fmt"
	"net"

	tenetv1beta2 "github.com/cybozu-go/tenet/api/v1beta2"
	"github.com/cybozu-go/tenet/pkg/cilium"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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

func (v *ciliumNetworkPolicyValidator) gatherIPFilters(nparl *tenetv1beta2.NetworkPolicyAdmissionRuleList, ls map[string]string) ([]*net.IPNet, []*net.IPNet, error) {
	var egressFilters, ingressFilters []*net.IPNet
	for _, npar := range nparl.Items {
		if matched, err := v.shouldExclude(&npar, ls); err != nil {
			return nil, nil, err
		} else if matched {
			continue
		}

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

func (v *ciliumNetworkPolicyValidator) gatherEntityPolicies(cnp *unstructured.Unstructured) ([]string, []string, error) {
	return v.gatherPolicies(cnp, cilium.EntityRuleKey, v.gatherPoliciesFromStringRule)
}

func (v *ciliumNetworkPolicyValidator) gatherEntityFilters(nparl *tenetv1beta2.NetworkPolicyAdmissionRuleList, ls map[string]string) ([]string, []string, error) {
	var egressFilters, ingressFilters []string
	for _, npar := range nparl.Items {
		if matched, err := v.shouldExclude(&npar, ls); err != nil {
			return nil, nil, err
		} else if matched {
			continue
		}

		for _, entity := range npar.Spec.ForbiddenEntities {
			switch entity.Type {
			case tenetv1beta2.NetworkPolicyAdmissionRuleTypeAll:
				egressFilters = append(egressFilters, entity.Entity)
				ingressFilters = append(ingressFilters, entity.Entity)
			case tenetv1beta2.NetworkPolicyAdmissionRuleTypeEgress:
				egressFilters = append(egressFilters, entity.Entity)
			case tenetv1beta2.NetworkPolicyAdmissionRuleTypeIngress:
				ingressFilters = append(ingressFilters, entity.Entity)
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
