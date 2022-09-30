package hooks

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenetv1beta2 "github.com/cybozu-go/tenet/api/v1beta2"
	"github.com/cybozu-go/tenet/pkg/cilium"
)

var (
	//go:embed t/allowed-cidr.yaml
	allowedCIDR []byte
	//go:embed t/egress-forbidden-cidrset.yaml
	egressForbiddenCIDRSet []byte
	//go:embed t/egress-forbidden-cidr.yaml
	egressForbiddenCIDR []byte
	//go:embed t/egress-forbidden-entity.yaml
	egressForbiddenEntity []byte
	//go:embed t/ingress-forbidden-cidrset.yaml
	ingressForbiddenCIDRSet []byte
	//go:embed t/ingress-forbidden-cidr.yaml
	ingressForbiddenCIDR []byte
	//go:embed t/ingress-forbidden-entity.yaml
	ingressForbiddenEntity []byte
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
		npar := &tenetv1beta2.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name: "default-rule",
			},
			Spec: tenetv1beta2.NetworkPolicyAdmissionRuleSpec{
				NamespaceSelector: tenetv1beta2.NetworkPolicyAdmissionRuleNamespaceSelector{
					ExcludeLabels: map[string]string{
						"team": "neco",
					},
				},
				ForbiddenIPRanges: []tenetv1beta2.NetworkPolicyAdmissionRuleForbiddenIPRanges{
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
				ForbiddenEntities: []tenetv1beta2.NetworkPolicyAdmissionRuleForbiddenEntity{
					{
						Entity: "host",
						Type:   "egress",
					},
					{
						Entity: "remote-node",
						Type:   "egress",
					},
					{
						Entity: "world",
						Type:   "all",
					},
				},
			},
		}
		err := k8sClient.Create(ctx, npar)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			npar := &tenetv1beta2.NetworkPolicyAdmissionRule{}
			key := client.ObjectKey{
				Name: "default-rule",
			}
			return k8sClient.Get(ctx, key, npar)
		}).Should(Succeed())
	})

	AfterEach(func() {
		err := k8sClient.DeleteAllOf(ctx, &tenetv1beta2.NetworkPolicyAdmissionRule{})
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

		cases := []struct {
			name     string
			manifest []byte
		}{
			{
				name:     "egress with forbidden CIDR",
				manifest: egressForbiddenCIDR,
			},
			{
				name:     "egress with forbidden CIDRSet",
				manifest: egressForbiddenCIDRSet,
			},
			{
				name:     "egress with forbidden entity",
				manifest: egressForbiddenEntity,
			},
		}
		for _, tc := range cases {
			By(fmt.Sprintf("applying %s", tc.name))
			Expect(createCiliumNetworkPolicy(ctx, nsName, tc.manifest)).To(HaveOccurred())
		}
	})

	It("should reject CiliumNetworkPolicies with forbidden ingress definition", func() {
		nsName := uuid.NewString()
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		cases := []struct {
			name     string
			manifest []byte
		}{
			{
				name:     "ingress with forbidden CIDR",
				manifest: ingressForbiddenCIDR,
			},
			{
				name:     "ingress with forbidden CIDRSet",
				manifest: ingressForbiddenCIDRSet,
			},
			{
				name:     "ingress with forbidden entity",
				manifest: ingressForbiddenEntity,
			},
		}
		for _, tc := range cases {
			By(fmt.Sprintf("applying %s", tc.name))
			Expect(createCiliumNetworkPolicy(ctx, nsName, tc.manifest)).To(HaveOccurred())
		}
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
				APIVersion: tenetv1beta2.GroupVersion.String(),
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
