package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	tenetv1beta2 "github.com/cybozu-go/tenet/api/v1beta2"
	"github.com/cybozu-go/tenet/pkg/cilium"
	cacheclient "github.com/cybozu-go/tenet/pkg/client"
	"github.com/cybozu-go/tenet/pkg/tenet"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
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
	intraNSCCNPTemplate = `
apiVersion: cilium.io/v2
kind: CiliumClusterwideNetworkPolicy
spec:
    endpointSelector:
        matchLabels:
          k8s:io.kubernetes.pod.namespace: {{.Name}}
    ingress:
    - fromEndpoints:
        - matchLabels:
            "k8s.io.cilium.k8s.namespace.labels.team": {{ index .Labels "team" }}
`
	expectedCNPTemplate = `
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
spec:
    endpointSelector: {}
    egress:
    - toEndpoints:
        - matchLabels:
            "k8s:io.kubernetes.pod.namespace": %s
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

func newDummyNetworkPolicyTemplate(o client.ObjectKey, tmpl string) *tenetv1beta2.NetworkPolicyTemplate {
	return &tenetv1beta2.NetworkPolicyTemplate{
		ObjectMeta: v1.ObjectMeta{
			Name: o.Name,
		},
		Spec: tenetv1beta2.NetworkPolicyTemplateSpec{
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
		ns.SetAnnotations(map[string]string{tenet.PolicyAnnotation: strings.Join(npts, ",")})
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
			NewClient:          cacheclient.NewCachingClient,
		})
		Expect(err).NotTo(HaveOccurred())

		nptr := &NetworkPolicyTemplateReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("NetworkPolicyTemplate"),
			Scheme: mgr.GetScheme(),
		}
		err = nptr.SetupWithManager(ctx, mgr)
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

		var cnp *unstructured.Unstructured
		Eventually(func() error {
			cnp = cilium.CiliumNetworkPolicy()
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		expectedCNPString := fmt.Sprintf(expectedCNPTemplate, nsName)
		expectedCNP := cilium.CiliumNetworkPolicy()
		y := yaml.NewYAMLOrJSONDecoder(strings.NewReader(expectedCNPString), len(expectedCNPString))
		err := y.Decode(expectedCNP)
		Expect(err).NotTo(HaveOccurred())
		Expect(equality.Semantic.DeepEqual(cnp.UnstructuredContent()["spec"], expectedCNP.UnstructuredContent()["spec"])).To(BeTrue())

		Eventually(func() tenetv1beta2.NetworkPolicyTemplateStatus {
			npt := &tenetv1beta2.NetworkPolicyTemplate{}
			nptKey := client.ObjectKey{
				Name: nptName,
			}
			err := k8sClient.Get(ctx, nptKey, npt)
			Expect(err).NotTo(HaveOccurred())
			return npt.Status
		}).Should(Equal(tenetv1beta2.NetworkPolicyTemplateOK))
	})

	It("should leave opted-out namespaces alone", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{})

		Consistently(func() error {
			cnp := cilium.CiliumNetworkPolicy()
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
	})

	It("should create CiliumClusterwideNetworkPolicy", func() {
		nptName := uuid.NewString()[:16]
		nsName := uuid.NewString()[:16]

		npt := newDummyNetworkPolicyTemplate(client.ObjectKey{Name: nptName}, intraNSCCNPTemplate)
		npt.Spec.ClusterWide = true
		err := k8sClient.Create(ctx, npt)
		Expect(err).NotTo(HaveOccurred())

		ns := &corev1.Namespace{}
		ns.Name = nsName

		ns.SetAnnotations(map[string]string{tenet.PolicyAnnotation: nptName})
		ns.SetLabels(map[string]string{"team": "my-team"})

		err = k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			ccnp := cilium.CiliumClusterwideNetworkPolicy()
			key := client.ObjectKey{
				Name: fmt.Sprintf("%s-%s", nsName, nptName),
			}
			return k8sClient.Get(ctx, key, ccnp)
		}).Should(Succeed())
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
				cnp := cilium.CiliumNetworkPolicy()
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

		var cnp *unstructured.Unstructured
		Eventually(func() error {
			cnp = cilium.CiliumNetworkPolicy()
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		npt := &tenetv1beta2.NetworkPolicyTemplate{}
		nptKey := client.ObjectKey{
			Name: nptName,
		}
		err := k8sClient.Get(ctx, nptKey, npt)
		Expect(err).NotTo(HaveOccurred())
		npt.Spec.PolicyTemplate = bmcDenyTemplate
		err = k8sClient.Update(ctx, npt)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			cnp = cilium.CiliumNetworkPolicy()
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      npt.Name,
			}
			if err := k8sClient.Get(ctx, key, cnp); err != nil {
				return err
			}
			_, hasEgressDeny, err := unstructured.NestedSlice(cnp.UnstructuredContent(), "spec", "egressDeny")
			Expect(err).NotTo(HaveOccurred())
			_, hasEgress, err := unstructured.NestedSlice(cnp.UnstructuredContent(), "spec", "egress")
			Expect(err).NotTo(HaveOccurred())
			if !hasEgressDeny && !hasEgress {
				return fmt.Errorf("wrong CiliumNetworkPolicy spec. Expected %v got %v", npt.Spec.PolicyTemplate, cnp.UnstructuredContent()["spec"])
			}
			return nil
		}).Should(Succeed())
	})

	It("should reconcile generated CiliumNetworkPolicies upon tenant edit", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		var cnp *unstructured.Unstructured
		Eventually(func() error {
			cnp = cilium.CiliumNetworkPolicy()
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())
		unstructured.RemoveNestedField(cnp.UnstructuredContent(), "spec", "egress")
		err := k8sClient.Update(ctx, cnp)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			cnp = cilium.CiliumNetworkPolicy()
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			err := k8sClient.Get(ctx, key, cnp)
			Expect(err).NotTo(HaveOccurred())
			_, hasEgress, err := unstructured.NestedSlice(cnp.UnstructuredContent(), "spec", "egress")
			if !hasEgress || err != nil {
				return fmt.Errorf("CiliumNetworkPolicy not reconciled")
			}
			return nil
		}).Should(Succeed())
	})

	It("should cleanup CiliumNetworkPolicy upon opt-out", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, intraNSTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		cnp := cilium.CiliumNetworkPolicy()
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

		npt := &tenetv1beta2.NetworkPolicyTemplate{}
		nptKey := client.ObjectKey{
			Name: nptName,
		}
		err := k8sClient.Get(ctx, nptKey, npt)
		Expect(err).NotTo(HaveOccurred())

		cnp := cilium.CiliumNetworkPolicy()
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

	It("should not delete other resources when cascade deleting", func() {
		intraNSNptName := uuid.NewString()
		bmcNptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, intraNSNptName, intraNSTemplate)
		shouldCreateNetworkPolicyTemplate(ctx, bmcNptName, bmcDenyTemplate)
		shouldCreateNamespace(ctx, nsName, []string{intraNSNptName, bmcNptName})

		npt := &tenetv1beta2.NetworkPolicyTemplate{}
		nptKey := client.ObjectKey{
			Name: intraNSNptName,
		}
		err := k8sClient.Get(ctx, nptKey, npt)
		Expect(err).NotTo(HaveOccurred())

		intraNSCNP := cilium.CiliumNetworkPolicy()
		intraNSKey := client.ObjectKey{
			Namespace: nsName,
			Name:      intraNSNptName,
		}
		bmcDenyCNP := cilium.CiliumNetworkPolicy()
		bmcDenyKey := client.ObjectKey{
			Namespace: nsName,
			Name:      bmcNptName,
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, intraNSKey, intraNSCNP)
		}).Should(Succeed())
		Consistently(func() error {
			return k8sClient.Get(ctx, intraNSKey, intraNSCNP)
		}).Should(Succeed())

		Eventually(func() error {
			return k8sClient.Get(ctx, bmcDenyKey, bmcDenyCNP)
		}).Should(Succeed())

		err = k8sClient.Delete(ctx, npt)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Get(ctx, intraNSKey, intraNSCNP)
		}).ShouldNot(Succeed())
		Consistently(func() error {
			return k8sClient.Get(ctx, intraNSKey, intraNSCNP)
		}).ShouldNot(Succeed())

		Consistently(func() error {
			return k8sClient.Get(ctx, bmcDenyKey, bmcDenyCNP)
		}).Should(Succeed())
	})

	It("should update status of invalid templates", func() {
		nptName := uuid.NewString()
		nsName := uuid.NewString()
		shouldCreateNetworkPolicyTemplate(ctx, nptName, invalidTemplate)
		shouldCreateNamespace(ctx, nsName, []string{nptName})

		Eventually(func() tenetv1beta2.NetworkPolicyTemplateStatus {
			npt := &tenetv1beta2.NetworkPolicyTemplate{}
			nptKey := client.ObjectKey{
				Name: nptName,
			}
			err := k8sClient.Get(ctx, nptKey, npt)
			Expect(err).NotTo(HaveOccurred())
			return npt.Status
		}).Should(Equal(tenetv1beta2.NetworkPolicyTemplateInvalid))

		Consistently(func() error {
			cnp := cilium.CiliumNetworkPolicy()
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      nptName,
			}
			return k8sClient.Get(ctx, key, cnp)
		}).ShouldNot(Succeed())
	})
})
