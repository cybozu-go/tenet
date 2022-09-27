package e2e

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/cybozu-go/tenet/pkg/cilium"
	"github.com/cybozu-go/tenet/pkg/tenet"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	allowIntraNSEgressPolicyName = "allow-intra-namespace-egress"
	clusterwideNPTName           = "clusterwide-npt"
	bmcDenyPolicyName            = "bmc-deny"
	dummyPolicyName              = "dummy"
)

var (
	//go:embed t/dummy-npt.yaml
	dummyNetworkPolicyTemplate []byte

	//go:embed t/user-edit-cnp.yaml
	userEditedCiliumNetworkPolicy []byte

	//go:embed t/bmc-allow-cnp.yaml
	bmcAllowCiliumNetworkPolicy []byte

	//go:embed t/node-entity-allow-cnp.yaml
	nodeEntityAllowCiliumNetworkPolicy []byte

	//go:embed t/legal-cnp.yaml
	legalCiliumNetworkPolicy []byte
)

func getCNPInNamespace(name, namespace string) error {
	_, err := kubectl(nil, "get", "-n", namespace, "CiliumNetworkPolicy", name)
	return err
}

func checkCNPCount(namespace string, count int) error {
	out, err := kubectl(nil, "get", "-n", namespace, "CiliumNetworkPolicy", "-o", "json")
	if err != nil {
		return err
	}
	cnpl := cilium.CiliumNetworkPolicyList()
	if err := json.Unmarshal(out, cnpl); err != nil {
		return err
	}
	if len(cnpl.Items) != count {
		return fmt.Errorf("expected exactly %d CiliumNetworkPolicies", count)
	}
	return nil
}

var _ = Describe("NetworkPolicyTemplate", func() {
	It("should create CiliumNetworkPolicy in opted-in namespace", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s", tenet.PolicyAnnotation, allowIntraNSEgressPolicyName))

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())

		Consistently(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())
	})

	It("should create CiliumNetworkPolicies for all opted-in templates", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s,%s", tenet.PolicyAnnotation, allowIntraNSEgressPolicyName, bmcDenyPolicyName))

		By("checking propagation")
		Eventually(func() error {
			return checkCNPCount(nsName, 2)
		}).Should(Succeed())
		Consistently(func() error {
			return checkCNPCount(nsName, 2)
		}).Should(Succeed())
	})

	It("should not create CiliumNetworkPolicy in regular namespace", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "label", "ns", nsName, "team=neco")

		By("checking non-propagation")
		Consistently(func() error {
			return checkCNPCount(nsName, 0)
		}).Should(Succeed())
	})

	It("should create CiliumClusterwideNetworkPolicy", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "label", "ns", nsName, "team=my-team")
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s", tenet.PolicyAnnotation, clusterwideNPTName))

		By("checking propagation")
		Consistently(func() error {
			_, err := kubectl(nil, "get", "CiliumClusterwideNetworkPolicy", fmt.Sprintf("%s-clusterwide-npt", nsName))
			return err
		}).Should(Succeed())
	})

	It("should prevent deletion of managed CiliumNetworkPolicies", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s", tenet.PolicyAnnotation, allowIntraNSEgressPolicyName))

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())

		By("preventing deletion of CiliumNetworkPolicy")
		_, err := kubectl(nil, "delete", "-n", nsName, "CiliumNetworkPolicy", allowIntraNSEgressPolicyName)
		Expect(err).To(HaveOccurred())
	})

	It("should allow deletion of unmanaged CiliumNetworkPolicies", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)

		By("applying user CiliumNetworkPolicy")
		kubectlSafe(userEditedCiliumNetworkPolicy, "apply", "-n", nsName, "-f", "-")

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())

		By("allowing deletion of CiliumNetworkPolicy")
		_, err := kubectl(nil, "delete", "-n", nsName, "CiliumNetworkPolicy", allowIntraNSEgressPolicyName)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should delete generated resources upon opt-out", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s", tenet.PolicyAnnotation, allowIntraNSEgressPolicyName))

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())

		By("opting out of NetworkPolicyTemplate")
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s-", tenet.PolicyAnnotation))

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).ShouldNot(Succeed())
	})

	It("should reconcile user edits to managed CiliumNetworkPolicy", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s", tenet.PolicyAnnotation, allowIntraNSEgressPolicyName))

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())

		By("applying user modifications to CiliumNetworkPolicy")
		kubectlSafe(userEditedCiliumNetworkPolicy, "apply", "-n", nsName, "-f", "-")

		y := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(userEditedCiliumNetworkPolicy), len(userEditedCiliumNetworkPolicy))
		userCNP := cilium.CiliumNetworkPolicy()
		err := y.Decode(userCNP)
		Expect(err).NotTo(HaveOccurred())

		var checkCNPSpec = func() error {
			out, err := kubectl(nil, "get", "-n", nsName, "CiliumNetworkPolicy", allowIntraNSEgressPolicyName, "-o", "json")
			if err != nil {
				return err
			}
			cnp := cilium.CiliumNetworkPolicy()
			if err := json.Unmarshal(out, cnp); err != nil {
				return err
			}
			if equality.Semantic.DeepEqual(cnp.UnstructuredContent()["spec"], userCNP.UnstructuredContent()["spec"]) {
				return fmt.Errorf("CiliumNetworkPolicy has not been reconciled")
			}
			return nil
		}

		By("checking propagation")
		Eventually(checkCNPSpec).Should(Succeed())
		Consistently(checkCNPSpec).Should(Succeed())
	})

	It("should cascade delete when finalizing", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s", tenet.PolicyAnnotation, dummyPolicyName))

		By("setting up NetworkPolicyTemplate")
		kubectlSafe(dummyNetworkPolicyTemplate, "apply", "-f", "-")

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).Should(Succeed())

		By("deleting NetworkPolicyTemplate")
		_, err := kubectl(nil, "delete", "NetworkPolicyTemplate", dummyPolicyName)
		Expect(err).NotTo(HaveOccurred())

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).ShouldNot(Succeed())
		Consistently(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).ShouldNot(Succeed())
	})

	It("should not delete other resoruces when cascade deleting", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "annotate", "ns", nsName, fmt.Sprintf("%s=%s,%s", tenet.PolicyAnnotation, dummyPolicyName, allowIntraNSEgressPolicyName))

		By("setting up NetworkPolicyTemplate")
		kubectlSafe(dummyNetworkPolicyTemplate, "apply", "-f", "-")

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).Should(Succeed())
		Eventually(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())

		By("deleting NetworkPolicyTemplate")
		_, err := kubectl(nil, "delete", "NetworkPolicyTemplate", dummyPolicyName)
		Expect(err).NotTo(HaveOccurred())

		By("checking propagation")
		Eventually(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).ShouldNot(Succeed())
		Consistently(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).ShouldNot(Succeed())
		Consistently(func() error {
			return getCNPInNamespace(allowIntraNSEgressPolicyName, nsName)
		}).Should(Succeed())
	})
})

var _ = Describe("NetworkPolicyAdmissionRule", func() {
	It("should reject a CiliumNetworkPolicy with forbidden IP", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "label", "ns", nsName, "team=tenant")

		By("applying bmc-allow CiliumNetworkPolicy")
		_, err := kubectl(bmcAllowCiliumNetworkPolicy, "apply", "-n", nsName, "-f", "-")
		Expect(err).To(HaveOccurred())

		Consistently(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).ShouldNot(Succeed())
	})

	It("should reject a CiliumNetworkPolicy with forbidden entity", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "label", "ns", nsName, "team=tenant")

		By("applying node-allow CiliumNetworkPolicy")
		_, err := kubectl(nodeEntityAllowCiliumNetworkPolicy, "apply", "-n", nsName, "-f", "-")
		Expect(err).To(HaveOccurred())

		Consistently(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).ShouldNot(Succeed())
	})

	It("should not apply admission rules to excluded namespaces", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)
		kubectlSafe(nil, "label", "ns", nsName, "team=neco")

		By("applying bmc-allow CiliumNetworkPolicy")
		_, err := kubectl(bmcAllowCiliumNetworkPolicy, "apply", "-n", nsName, "-f", "-")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).Should(Succeed())
	})

	It("should not reject a legal CiliumNetworkPolicy", func() {
		By("setting up namespace")
		nsName := uuid.NewString()
		kubectlSafe(nil, "create", "ns", nsName)

		By("applying CiliumNetworkPolicy without forbidden IPs")
		_, err := kubectl(legalCiliumNetworkPolicy, "apply", "-n", nsName, "-f", "-")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			return getCNPInNamespace(dummyPolicyName, nsName)
		}).Should(Succeed())
	})
})
