package hooks

import (
	"bytes"
	"context"
	_ "embed"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenetv1beta1 "github.com/cybozu-go/tenet/api/v1beta1"
	"github.com/cybozu-go/tenet/pkg/cilium"
)

var (
	//go:embed t/allowed-cidr.yaml
	allowedCIDR []byte
	//go:embed t/egress-forbidden-cidrset.yaml
	egressForbiddenCIDRSet []byte
	//go:embed t/egress-forbidden-cidr.yaml
	egressForbiddenCIDR []byte
	//go:embed t/ingress-forbidden-cidrset.yaml
	ingressForbiddenCIDRSet []byte
	//go:embed t/ingress-forbidden-cidr.yaml
	ingressForbiddenCIDR []byte
	//go:embed t/either-forbidden.yaml
	eitherForbidden []byte
	//go:embed t/multiple-cnp.yaml
	multiplePolicySpecs []byte
)

func createCiliumNetworkPolicy(ctx context.Context, nsName string, contents []byte) error {
	y := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(contents), len(contents))
	cnp := cilium.CiliumNetworkPolicy()
	err := y.Decode(cnp)
	Expect(err).NotTo(HaveOccurred())
	cnp.SetNamespace(nsName)
	return k8sClient.Create(ctx, cnp)
}

var _ = Describe("CiliumNetworkPolicy webhook", func() {
	ctx := context.Background()

	BeforeEach(func() {
		npar := &tenetv1beta1.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name: "default-rule",
			},
			Spec: tenetv1beta1.NetworkPolicyAdmissionRuleSpec{
				NamespaceSelector: tenetv1beta1.NetworkPolicyAdmissionRuleNamespaceSelector{
					ExcludeLabels: map[string]string{
						"team": "neco",
					},
				},
				ForbiddenIPRanges: []tenetv1beta1.NetworkPolicyAdmissionRuleForbiddenIPRanges{
					{
						CIDR: "10.72.16.0/20",
						Type: "egress",
					},
					{
						CIDR: "10.76.16.0/20",
						Type: "ingress",
					},
					{
						CIDR: "10.78.16.0/20",
						Type: "all",
					},
				},
			},
		}
		err := k8sClient.Create(ctx, npar)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			npar := &tenetv1beta1.NetworkPolicyAdmissionRule{}
			key := client.ObjectKey{
				Name: "default-rule",
			}
			return k8sClient.Get(ctx, key, npar)
		}).Should(Succeed())
	})

	AfterEach(func() {
		err := k8sClient.DeleteAllOf(ctx, &tenetv1beta1.NetworkPolicyAdmissionRule{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not reject CiliumNetworkPolicies in excluded namespaces", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		ns.SetLabels(map[string]string{
			"team": "neco",
		})
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, egressForbiddenCIDRSet)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not reject CiliumNetworkPolicies without forbidden definitions", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, allowedCIDR)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should reject CiliumNetworkPolicies with forbidden egress definition", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		ns.SetLabels(map[string]string{
			"team": "tenant",
		})
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, egressForbiddenCIDR)
		Expect(err).To(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, egressForbiddenCIDRSet)
		Expect(err).To(HaveOccurred())
	})

	It("should reject CiliumNetworkPolicies with forbidden ingress definition", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, ingressForbiddenCIDR)
		Expect(err).To(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, ingressForbiddenCIDRSet)
		Expect(err).To(HaveOccurred())
	})

	It("should reject CiliumNetworkPolicies with forbidden ingress or egress definition", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, eitherForbidden)
		Expect(err).To(HaveOccurred())
	})

	It("should handle CiliumNetworkPolicies with multiple specs", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		err = createCiliumNetworkPolicy(ctx, nsName, multiplePolicySpecs)
		Expect(err).To(HaveOccurred())
	})

	It("should block user deletion of managed CiliumNetworkPolicies", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		y := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(allowedCIDR), len(allowedCIDR))
		cnp := cilium.CiliumNetworkPolicy()
		err = y.Decode(cnp)
		Expect(err).NotTo(HaveOccurred())
		cnp.SetNamespace(nsName)
		cnp.SetOwnerReferences([]v1.OwnerReference{
			{
				APIVersion: tenetv1beta1.GroupVersion.String(),
				Kind:       "NetworkPolicyTemplate",
				Name:       "dummy",
				UID:        types.UID(uuid.NewString()),
			},
		})
		err = k8sClient.Create(ctx, cnp)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      cnp.GetName(),
			}
			cnp = cilium.CiliumNetworkPolicy()
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		err = k8sClient.Delete(ctx, cnp)
		Expect(err).To(HaveOccurred())
	})

	It("should allow user deletion of unmanaged CiliumNetworkPolicies", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		y := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(allowedCIDR), len(allowedCIDR))
		cnp := cilium.CiliumNetworkPolicy()
		err = y.Decode(cnp)
		Expect(err).NotTo(HaveOccurred())
		cnp.SetNamespace(nsName)
		cnp.SetOwnerReferences([]v1.OwnerReference{})
		err = k8sClient.Create(ctx, cnp)
		Expect(err).NotTo(HaveOccurred())

		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			key := client.ObjectKey{
				Namespace: nsName,
				Name:      cnp.GetName(),
			}
			cnp = cilium.CiliumNetworkPolicy()
			return k8sClient.Get(ctx, key, cnp)
		}).Should(Succeed())

		err = k8sClient.Delete(ctx, cnp)
		Expect(err).NotTo(HaveOccurred())
	})
})
