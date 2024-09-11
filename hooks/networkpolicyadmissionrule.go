package hooks

import (
	"context"
	"net"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tenetv1beta2 "github.com/cybozu-go/tenet/api/v1beta2"
)

//+kubebuilder:webhook:path=/validate-tenet-cybozu-io-v1beta2-networkpolicyadmissionrule,mutating=false,failurePolicy=fail,sideEffects=None,groups=tenet.cybozu.io,resources=networkpolicyadmissionrules,verbs=create;update,versions=v1beta2,name=vnetworkpolicyadmissionrule.kb.io,admissionReviewVersions={v1}

type networkPolicyAdmissionRuleValidator struct {
	client.Client
	dec admission.Decoder
}

var _ admission.Handler = &networkPolicyAdmissionRuleValidator{}

// Handle validates the NetworkPolicyAdmissionRule.
func (v *networkPolicyAdmissionRuleValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	npar := &tenetv1beta2.NetworkPolicyAdmissionRule{}
	if err := v.dec.Decode(req, npar); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	for _, ipRange := range npar.Spec.ForbiddenIPRanges {
		if _, _, err := net.ParseCIDR(ipRange.CIDR); err != nil {
			return admission.Denied("a malformed CIDR string was provided")
		}
		if ipRange.Type == "" {
			return admission.Denied("a connection type must be provided")
		}
	}
	return admission.Allowed("")
}

func SetupNetworkPolicyAdmissionRuleWebhook(mgr manager.Manager, dec admission.Decoder) {
	v := &networkPolicyAdmissionRuleValidator{
		Client: mgr.GetClient(),
		dec:    dec,
	}
	srv := mgr.GetWebhookServer()
	srv.Register("/validate-tenet-cybozu-io-v1beta2-networkpolicyadmissionrule", &webhook.Admission{Handler: v})
}
