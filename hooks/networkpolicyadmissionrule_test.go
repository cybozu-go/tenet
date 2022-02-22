package hooks

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tenetv1beta2 "github.com/cybozu-go/tenet/api/v1beta2"
)

var _ = Describe("NetworkPolicyAdmissionRule webhook", func() {
	ctx := context.Background()

	It("should deny the creation of a NetworkPolicyAdmissionRule with malformed CIDR", func() {
		npar := &tenetv1beta2.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name: uuid.NewString(),
			},
			Spec: tenetv1beta2.NetworkPolicyAdmissionRuleSpec{
				ForbiddenIPRanges: []tenetv1beta2.NetworkPolicyAdmissionRuleForbiddenIPRanges{
					{
						CIDR: "300.300.300.0/12",
						Type: "all",
					},
				},
			},
		}
		err := k8sClient.Create(ctx, npar)
		Expect(err).To(HaveOccurred())
	})

	It("should deny the creation of a NetworkPolicyAdmissionRule without connection type", func() {
		npar := &tenetv1beta2.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: "default",
			},
			Spec: tenetv1beta2.NetworkPolicyAdmissionRuleSpec{
				ForbiddenIPRanges: []tenetv1beta2.NetworkPolicyAdmissionRuleForbiddenIPRanges{
					{
						CIDR: "10.0.0.0/24",
					},
				},
			},
		}
		err := k8sClient.Create(ctx, npar)
		Expect(err).To(HaveOccurred())
	})

	It("should allow valid NetworkPolicyAdmissionRules", func() {
		npar := &tenetv1beta2.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: "default",
			},
			Spec: tenetv1beta2.NetworkPolicyAdmissionRuleSpec{
				ForbiddenIPRanges: []tenetv1beta2.NetworkPolicyAdmissionRuleForbiddenIPRanges{
					{
						CIDR: "10.0.0.0/24",
						Type: "egress",
					},
				},
			},
		}
		err := k8sClient.Create(ctx, npar)
		Expect(err).NotTo(HaveOccurred())
	})
})
