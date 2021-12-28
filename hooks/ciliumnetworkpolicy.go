package hooks

import (
	"context"
	"net"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/policy/api"
	tenetv1beta1 "github.com/cybozu-go/tenet/api/v1beta1"
)

//+kubebuilder:webhook:path=/validate-cilium-io-v2-ciliumnetworkpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=cilium.io,resources=ciliumnetworkpolicies,verbs=create;update,versions=v2,name=vciliumnetworkpolicy.kb.io,admissionReviewVersions={v1}

type ciliumNetworkPolicyValidator struct {
	client.Client
	dec *admission.Decoder
}

var _ admission.Handler = &ciliumNetworkPolicyValidator{}

// Handler validates CiliumNetworkPolicies.
func (v *ciliumNetworkPolicyValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	cnp := &ciliumv2.CiliumNetworkPolicy{}
	if err := v.dec.Decode(req, cnp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	ns := &corev1.Namespace{}
	if err := v.Get(ctx, client.ObjectKey{Name: cnp.Namespace}, ns); client.IgnoreNotFound(err) != nil {
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

func (v *ciliumNetworkPolicyValidator) gatherPolicies(cnp *ciliumv2.CiliumNetworkPolicy) ([]*net.IPNet, []*net.IPNet, error) {
	egressPolicies := []*net.IPNet{}
	ingressPolicies := []*net.IPNet{}
	if cnp.Spec != nil {
		e, i, err := v.gatherPoliciesFromRule(cnp.Spec)
		if err != nil {
			return nil, nil, err
		}
		egressPolicies = append(egressPolicies, e...)
		ingressPolicies = append(ingressPolicies, i...)
	}
	if cnp.Specs != nil {
		for _, rule := range cnp.Specs {
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

func (v *ciliumNetworkPolicyValidator) gatherPoliciesFromRule(rule *api.Rule) ([]*net.IPNet, []*net.IPNet, error) {
	egressPolicies, err := v.gatherEgressPolicies(rule)
	if err != nil {
		return nil, nil, err
	}
	ingressPolicies, err := v.gatherIngressPolicies(rule)
	if err != nil {
		return nil, nil, err
	}
	return egressPolicies, ingressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherEgressPolicies(rule *api.Rule) ([]*net.IPNet, error) {
	if rule.Egress == nil {
		return nil, nil
	}
	egressPolicies := []*net.IPNet{}
	for _, e := range rule.Egress {
		for _, cidrString := range e.ToCIDR {
			_, cidr, err := net.ParseCIDR(string(cidrString))
			if err != nil {
				return nil, err
			}
			egressPolicies = append(egressPolicies, cidr)
		}
		for _, cidrString := range e.ToCIDRSet {
			_, cidr, err := net.ParseCIDR(string(cidrString.Cidr))
			if err != nil {
				return nil, err
			}
			egressPolicies = append(egressPolicies, cidr)
		}
	}
	return egressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherIngressPolicies(rule *api.Rule) ([]*net.IPNet, error) {
	if rule.Ingress == nil {
		return nil, nil
	}
	ingressPolicies := []*net.IPNet{}
	for _, i := range rule.Ingress {
		for _, cidrString := range i.FromCIDR {
			_, cidr, err := net.ParseCIDR(string(cidrString))
			if err != nil {
				return nil, err
			}
			ingressPolicies = append(ingressPolicies, cidr)
		}
		for _, cidrString := range i.FromCIDRSet {
			_, cidr, err := net.ParseCIDR(string(cidrString.Cidr))
			if err != nil {
				return nil, err
			}
			ingressPolicies = append(ingressPolicies, cidr)
		}
	}
	return ingressPolicies, nil
}

func (v *ciliumNetworkPolicyValidator) gatherFilters(nparl *tenetv1beta1.NetworkPolicyAdmissionRuleList) ([]*net.IPNet, []*net.IPNet, error) {
	egressFilters := []*net.IPNet{}
	ingressFilters := []*net.IPNet{}
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
