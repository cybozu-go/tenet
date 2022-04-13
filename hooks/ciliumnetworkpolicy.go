package hooks

import (
	"context"
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

	return v.validateEntity(nparl, cnp)
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

func (v *ciliumNetworkPolicyValidator) validateEntity(nparl tenetv1beta2.NetworkPolicyAdmissionRuleList, cnp *unstructured.Unstructured) admission.Response {
	egressPolicies, ingressPolicies, err := v.gatherEntityPolicies(cnp)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	egressFilters, ingressFilters := v.gatherEntityFilters(&nparl)
	for _, egressPolicy := range egressPolicies {
		for _, egressFilter := range egressFilters {
			if egressPolicy == egressFilter {
				return admission.Denied("an egress policy is requesting a forbidden entity")
			}
		}
	}
	for _, ingressPolicy := range ingressPolicies {
		for _, ingressFilter := range ingressFilters {
			if ingressPolicy == ingressFilter {
				return admission.Denied("an ingress policy is requesting a forbidden entity")
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
