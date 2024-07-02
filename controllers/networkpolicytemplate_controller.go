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

package controllers

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	tenetv1beta2 "github.com/cybozu-go/tenet/api/v1beta2"
	"github.com/cybozu-go/tenet/pkg/cilium"
	"github.com/cybozu-go/tenet/pkg/tenet"
)

const (
	finalizerName = "tenet.cybozu.io/finalizer"
)

// NetworkPolicyTemplateReconciler reconciles a NetworkPolicyTemplate object.
type NetworkPolicyTemplateReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=tenet.cybozu.io,resources=networkpolicytemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=tenet.cybozu.io,resources=networkpolicytemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=tenet.cybozu.io,resources=networkpolicytemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=tenet.cybozu.io,resources=networkpolicyadmissionrules,verbs=get;list
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="cilium.io",resources=ciliumnetworkpolicies,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="cilium.io",resources=ciliumclusterwidenetworkpolicies,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *NetworkPolicyTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	npt := &tenetv1beta2.NetworkPolicyTemplate{}
	if err := r.Get(ctx, req.NamespacedName, npt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if npt.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(npt, finalizerName) {
			controllerutil.AddFinalizer(npt, finalizerName)
			if err := r.Update(ctx, npt); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		logger.Info("finalizing")
		if err := r.finalize(ctx, npt); err != nil {
			return ctrl.Result{}, fmt.Errorf("finalization failed: %w", err)
		}
		logger.Info("done finalizing")
		return ctrl.Result{}, nil
	}

	return r.reconcileTemplate(ctx, npt)
}

func (r *NetworkPolicyTemplateReconciler) shouldDelete(npt *tenetv1beta2.NetworkPolicyTemplate, ownerRefs []v1.OwnerReference) bool {
	for _, ownerRef := range ownerRefs {
		if ownerRef.APIVersion == tenetv1beta2.GroupVersion.String() && ownerRef.Kind == tenetv1beta2.NetworkPolicyTemplateKind && ownerRef.Name == npt.Name {
			return true
		}
	}
	return false
}

func (r *NetworkPolicyTemplateReconciler) finalize(ctx context.Context, npt *tenetv1beta2.NetworkPolicyTemplate) error {
	if !controllerutil.ContainsFinalizer(npt, finalizerName) {
		return nil
	}

	logger := log.FromContext(ctx)

	var npl *unstructured.UnstructuredList
	if npt.Spec.ClusterWide {
		npl = cilium.CiliumClusterwideNetworkPolicyList()
	} else {
		npl = cilium.CiliumNetworkPolicyList()
	}
	if err := r.List(ctx, npl); client.IgnoreNotFound(err) != nil {
		return err
	}
	for _, np := range npl.Items {
		if np.GetDeletionTimestamp() != nil {
			continue
		}
		if !r.shouldDelete(npt, np.GetOwnerReferences()) {
			continue
		}
		if err := r.Delete(ctx, &np); err != nil {
			return fmt.Errorf("failed to delete %s %s: %w", np.GetKind(), np.GetName(), err)
		}
		logger.Info("deleted NetworkPolicy", "name", np.GetName(), "kind", np.GetKind())
	}

	controllerutil.RemoveFinalizer(npt, finalizerName)
	return r.Update(ctx, npt)
}

func (r *NetworkPolicyTemplateReconciler) reconcileTemplate(ctx context.Context, npt *tenetv1beta2.NetworkPolicyTemplate) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	npt.Status = tenetv1beta2.NetworkPolicyTemplateOK

	nsl := &corev1.NamespaceList{}
	if err := r.List(ctx, nsl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	for _, ns := range nsl.Items {
		if err := r.reconcileNetworkPolicy(ctx, npt, ns); err != nil {
			logger.Error(err, "failed to reconcile namespace", "name", ns.Name)
		}
	}

	if err := r.Status().Update(ctx, npt); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile template: %w", err)
	}

	logger.Info("done reconciling")
	return ctrl.Result{}, nil
}

func (r *NetworkPolicyTemplateReconciler) reconcileNetworkPolicy(ctx context.Context, npt *tenetv1beta2.NetworkPolicyTemplate, ns corev1.Namespace) error {
	logger := log.FromContext(ctx)

	var existingNetworkPolicy *unstructured.Unstructured
	var existingNetworkPolicyObjectKey types.NamespacedName
	if npt.Spec.ClusterWide {
		existingNetworkPolicy = cilium.CiliumClusterwideNetworkPolicy()
		existingNetworkPolicyObjectKey = client.ObjectKey{
			Name: fmt.Sprintf("%s-%s", ns.Name, npt.Name),
		}
	} else {
		existingNetworkPolicy = cilium.CiliumNetworkPolicy()
		existingNetworkPolicyObjectKey = client.ObjectKey{
			Namespace: ns.Name,
			Name:      npt.Name,
		}
	}

	existingNetworkPolicyError := r.Get(ctx, existingNetworkPolicyObjectKey, existingNetworkPolicy)
	if client.IgnoreNotFound(existingNetworkPolicyError) != nil {
		return existingNetworkPolicyError
	}

	// delete networkpolicy if the namespace no longer opts-in to it
	if !r.isOptedIntoTemplate(npt, ns) {
		if apierrors.IsNotFound(existingNetworkPolicyError) {
			return nil
		}
		return r.Delete(ctx, existingNetworkPolicy)
	}

	currentNetworkPolicy, err := r.compileTemplate(npt, ns)
	if err != nil {
		npt.Status = tenetv1beta2.NetworkPolicyTemplateInvalid
		logger.Error(err, "invalid template", "name", npt.Name)
		return err
	}
	if apierrors.IsNotFound(existingNetworkPolicyError) {
		logger.Info("creating NetworkPolicy", "name", currentNetworkPolicy.GetName(), "kind", currentNetworkPolicy.GetKind())
		return r.Create(ctx, currentNetworkPolicy)
	}
	if equality.Semantic.DeepEqual(existingNetworkPolicy.UnstructuredContent()["spec"], currentNetworkPolicy.UnstructuredContent()["spec"]) {
		return nil
	}
	existingNetworkPolicy.UnstructuredContent()["spec"] = currentNetworkPolicy.DeepCopy().UnstructuredContent()["spec"]
	logger.Info("updating NetworkPolicy", "name", existingNetworkPolicy.GetName(), "kind", currentNetworkPolicy.GetKind())
	return r.Update(ctx, existingNetworkPolicy)
}

func (r *NetworkPolicyTemplateReconciler) isOptedIntoTemplate(npt *tenetv1beta2.NetworkPolicyTemplate, ns corev1.Namespace) bool {
	for _, a := range strings.Split(ns.Annotations[tenet.PolicyAnnotation], ",") {
		if a == npt.Name {
			return true
		}
	}
	return false
}

func (r *NetworkPolicyTemplateReconciler) compileTemplate(npt *tenetv1beta2.NetworkPolicyTemplate, ns corev1.Namespace) (*unstructured.Unstructured, error) {
	var np, refNP *unstructured.Unstructured
	if npt.Spec.ClusterWide {
		np = cilium.CiliumClusterwideNetworkPolicy()
		refNP = cilium.CiliumClusterwideNetworkPolicy()
	} else {
		np = cilium.CiliumNetworkPolicy()
		refNP = cilium.CiliumNetworkPolicy()
	}
	tpl, err := template.New(npt.Name).Parse(npt.Spec.PolicyTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ns.ObjectMeta); err != nil {
		return nil, err
	}
	y := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(buf.Bytes()), buf.Len())
	if err := y.Decode(np); err != nil {
		return nil, err
	}
	if np.GetAPIVersion() != refNP.GetAPIVersion() || np.GetKind() != refNP.GetKind() {
		return nil, fmt.Errorf("invalid schema: %v", np.GetObjectKind().GroupVersionKind())
	}

	if npt.Spec.ClusterWide {
		np.SetName(fmt.Sprintf("%s-%s", ns.Name, npt.Name))
	} else {
		np.SetNamespace(ns.Name)
		np.SetName(npt.Name)
	}
	if err := controllerutil.SetOwnerReference(npt, np, r.Scheme); err != nil {
		return nil, err
	}
	return np, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkPolicyTemplateReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	logger := log.FromContext(ctx)
	getOwner := func(o client.Object, t *unstructured.Unstructured) []string {
		if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(o), t); err != nil {
			logger.Error(err, "failed to get CiliumNetworkPolicy")
			return nil
		}
		owners := t.GetOwnerReferences()
		for _, owner := range owners {
			if owner.APIVersion == tenetv1beta2.GroupVersion.String() && owner.Kind == tenetv1beta2.NetworkPolicyTemplateKind {
				return []string{owner.Name}
			}
		}
		return nil
	}
	listNPTs := func(ctx context.Context, _ client.Object) []reconcile.Request {
		var nptl tenetv1beta2.NetworkPolicyTemplateList
		if err := r.List(ctx, &nptl); err != nil {
			r.Log.Error(err, "failed to list NetworkPolicyTemplates")
			return nil
		}

		requests := make([]reconcile.Request, len(nptl.Items))
		for i, npt := range nptl.Items {
			requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{
				Name: npt.Name,
			}}
		}
		return requests
	}

	filterCNP := func(ctx context.Context, o client.Object) []reconcile.Request {
		if getOwner(o, cilium.CiliumNetworkPolicy()) == nil {
			return nil
		}
		return listNPTs(ctx, o)
	}

	filterCCNP := func(ctx context.Context, o client.Object) []reconcile.Request {
		if getOwner(o, cilium.CiliumClusterwideNetworkPolicy()) == nil {
			return nil
		}
		return listNPTs(ctx, o)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&tenetv1beta2.NetworkPolicyTemplate{}).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(listNPTs)).
		Watches(cilium.CiliumNetworkPolicy(), handler.EnqueueRequestsFromMapFunc(filterCNP)).
		Watches(cilium.CiliumClusterwideNetworkPolicy(), handler.EnqueueRequestsFromMapFunc(filterCCNP)).
		Complete(r)
}
