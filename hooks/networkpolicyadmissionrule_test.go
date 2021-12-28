package hooks

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tenetv1beta1 "github.com/cybozu-go/tenet/api/v1beta1"
)

var _ = Describe("NetworkPolicyAdmissionRule webhook", func() {
	ctx := context.Background()

	It("should deny the creation of a NetworkPolicyAdmissionRule with malformed CIDR", func() {
		npar := &tenetv1beta1.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name: uuid.NewString(),
			},
			Spec: tenetv1beta1.NetworkPolicyAdmissionRuleSpec{
				ForbiddenIPRanges: []tenetv1beta1.NetworkPolicyAdmissionRuleForbiddenIPRanges{
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
		npar := &tenetv1beta1.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: "default",
			},
			Spec: tenetv1beta1.NetworkPolicyAdmissionRuleSpec{
				ForbiddenIPRanges: []tenetv1beta1.NetworkPolicyAdmissionRuleForbiddenIPRanges{
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
		npar := &tenetv1beta1.NetworkPolicyAdmissionRule{
			ObjectMeta: v1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: "default",
			},
			Spec: tenetv1beta1.NetworkPolicyAdmissionRuleSpec{
				ForbiddenIPRanges: []tenetv1beta1.NetworkPolicyAdmissionRuleForbiddenIPRanges{
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
