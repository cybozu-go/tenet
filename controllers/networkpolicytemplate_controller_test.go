package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	tenetv1beta1 "github.com/cybozu-go/tenet/api/v1beta1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	intraNSTemplate = `
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
spec:
    endpointSelector: {}
    egress:
    - toEndpoints:
        - matchLabels:
            "k8s:io.kubernetes.pod.namespace": {{.Name}}
`
	bmcDenyTemplate = `
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
spec:
    endpointSelector: {}
    egressDeny:
    - toCIDRSet:
        - cidr: 10.72.16.0/20
        - cidr: 10.76.16.0/20
        - cidr: 10.78.16.0/20
`
	invalidTemplate = `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
spec:
    egress:
    - to:
      - ipBlock:
          cidr: 10.72.16.0/20
`
)

func newDummyNetworkPolicyTemplate(o client.ObjectKey, tmpl string) *tenetv1beta1.NetworkPolicyTemplate {
	return &tenetv1beta1.NetworkPolicyTemplate{
		ObjectMeta: v1.ObjectMeta{
			Name: o.Name,
		},
		Spec: tenetv1beta1.NetworkPolicyTemplateSpec{
			PolicyTemplate: tmpl,
		},
	}
}

func shouldCreateNetworkPolicyTemplate(ctx context.Context, nptName, tmpl string) {
	npt := newDummyNetworkPolicyTemplate(client.ObjectKey{Name: nptName}, tmpl)
	err := k8sClient.Create(ctx, npt)
	Expect(err).NotTo(HaveOccurred())
}

func shouldCreateNamespace(ctx context.Context, nsName string, npts []string) {
	ns := &corev1.Namespace{}
	ns.Name = nsName

	if len(npts) > 0 {
		ns.SetAnnotations(map[string]string{PolicyAnnotation: strings.Join(npts, ",")})
	}

	err := k8sClient.Create(ctx, ns)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Tenet controller", func() {
	ctx := context.Background()
	var stopFunc func()

	BeforeEach(func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).NotTo(HaveOccurred())

		nptr := &NetworkPolicyTemplateReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("NetworkPolicyTemplate"),
			Scheme: mgr.GetScheme(),
		}
		err = nptr.SetupWithManager(mgr)
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithCancel(ctx)
		stopFunc = cancel
		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		stopFunc()
		time.Sleep(100 * time.Millisecond)
	})

	It("should create CiliumNetworkPolicy in opted-in namespace", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		var cnp *ciliumv2.CiliumNetworkPolicy
		Eventually(func() error {
			cnp = &ciliumv2.CiliumNetworkPolicy{}
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		Expect(cnp.Spec.Egress[0].ToEndpoints[0].MatchLabels["k8s.io.kubernetes.pod.namespace"]).To(Equal(nsName))

		Eventually(func() tenetv1beta1.NetworkPolicyTemplateStatus {
			npt := &tenetv1beta1.NetworkPolicyTemplate{}
			nptKey := client.ObjectKey{
				Name: nptName,
			}
			err := k8sClient.Get(ctx, nptKey, npt)
			Expect(err).NotTo(HaveOccurred())
			return npt.Status
		}).Should(Equal(tenetv1beta1.NetworkPolicyTemplateOK))
	})

	It("should leave opted-out namespaces alone", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{})

		Consistently(func() error {
			cnp := &ciliumv2.CiliumNetworkPolicy{}
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
	})

	It("should apply all opted-in templates", func() {
		nsName := uuid.NewString()
		nptName1 := uuid.NewString()
		nptName2 := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName1, intraNSTemplate)
		shouldCreateNetworkPolicyTemplate(ctx, nptName2, bmcDenyTemplate)

		optins := []string{nptName1, nptName2}
		shouldCreateNamespace(ctx, nsName, optins)

		Eventually(func() error {
			for _, nptName := range optins {
				cnp := &ciliumv2.CiliumNetworkPolicy{}
				key := client.ObjectKey{
					Namespace: nsName,
					Name:      nptName,
				}
				if err := k8sClient.Get(ctx, key, cnp); err != nil {
					return err
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should cascade template updates to generated CiliumNetworkPolicies", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		var cnp *ciliumv2.CiliumNetworkPolicy
		Eventually(func() error {
			cnp = &ciliumv2.CiliumNetworkPolicy{}
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		npt := &tenetv1beta1.NetworkPolicyTemplate{}
		nptKey := client.ObjectKey{
			Name: nptName,
		}
		err := k8sClient.Get(ctx, nptKey, npt)
		Expect(err).NotTo(HaveOccurred())
		npt.Spec.PolicyTemplate = bmcDenyTemplate
		err = k8sClient.Update(ctx, npt)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			cnp = &ciliumv2.CiliumNetworkPolicy{}
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      npt.Name,
			}
			if err := k8sClient.Get(ctx, key, cnp); err != nil {
				return err
			}
			if cnp.Spec.EgressDeny == nil || cnp.Spec.Egress != nil {
				return fmt.Errorf("wrong CiliumNetworkPolicy spec. Expected %v got %v", npt.Spec.PolicyTemplate, cnp.Spec)
			}
			return nil
		}).Should(Succeed())
	})

	It("should reconcile generated CiliumNetworkPolicies upon tenant edit", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		var cnp *ciliumv2.CiliumNetworkPolicy
		Eventually(func() error {
			cnp = &ciliumv2.CiliumNetworkPolicy{}
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		cnp.Spec.Egress[0].ToEndpoints[0].MatchLabels["k8s.io.kubernetes.pod.namespace"] = "kube-system"
		err := k8sClient.Update(ctx, cnp)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() string {
			cnp = &ciliumv2.CiliumNetworkPolicy{}
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			err := k8sClient.Get(ctx, key, cnp)
			Expect(err).NotTo(HaveOccurred())
			return cnp.Spec.Egress[0].ToEndpoints[0].MatchLabels["k8s.io.kubernetes.pod.namespace"]
		}).Should(Equal(nsName))
	})

	It("should cleanup CiliumNetworkPolicy upon opt-out", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		cnp := &ciliumv2.CiliumNetworkPolicy{}
		key := client.ObjectKey{
			Namespace: nsName,
			Name:      nptName,
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())
		Consistently(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)
		Expect(err).NotTo(HaveOccurred())
		ns.SetAnnotations(map[string]string{})
		err = k8sClient.Update(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
		Consistently(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
	})

	It("should cascade delete when finalizing", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		npt := &tenetv1beta1.NetworkPolicyTemplate{}
		nptKey := client.ObjectKey{
			Name: nptName,
		}
		err := k8sClient.Get(ctx, nptKey, npt)
		Expect(err).NotTo(HaveOccurred())

		cnp := &ciliumv2.CiliumNetworkPolicy{}
		key := client.ObjectKey{
			Namespace: nsName,
			Name:      nptName,
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())
		Consistently(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		err = k8sClient.Delete(ctx, npt)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
		Consistently(func() error {
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
	})

	It("should update status of invalid templates", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, invalidTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		Eventually(func() tenetv1beta1.NetworkPolicyTemplateStatus {
			npt := &tenetv1beta1.NetworkPolicyTemplate{}
			nptKey := client.ObjectKey{
				Name: nptName,
			}
			err := k8sClient.Get(ctx, nptKey, npt)
			Expect(err).NotTo(HaveOccurred())
			return npt.Status
		}).Should(Equal(tenetv1beta1.NetworkPolicyTemplateInvalid))

		Consistently(func() error {
			cnp := &ciliumv2.CiliumNetworkPolicy{}
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
	})
})
