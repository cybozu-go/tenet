package e2e

import (
	_ "embed"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	//go:embed t/intra-namespace-npt.yaml
	intraNSNetworkPolicyTemplate []byte

	//go:embed t/bmc-deny-npt.yaml
	bmcDenyNetworkPolicyTemplate []byte

	//go:embed t/clusterwide-npt.yaml
	clusterwideNetworkPolicyTemplate []byte

	//go:embed t/bmc-deny-npar.yaml
	bmcDenyNetworkPolicyAdmissionRule []byte

	//go:embed t/node-deny-npar.yaml
	nodeDenyNetworkPolicyAdmissionRule []byte

	//go:embed t/exclude-only-npar.yaml
	excludeOnlyNetworkPolicyAdmissionRule []byte
)

func TestE2E(t *testing.T) {
	if !runE2E {
		t.Skip("no RUN_E2E environment variable")
	}
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(15 * time.Second)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultConsistentlyDuration(5 * time.Second)
	SetDefaultConsistentlyPollingInterval(1 * time.Second)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	By("setting up default NetworkPolicyTemplates")
	kubectlSafe(intraNSNetworkPolicyTemplate, "apply", "-f", "-")
	kubectlSafe(bmcDenyNetworkPolicyTemplate, "apply", "-f", "-")
	kubectlSafe(clusterwideNetworkPolicyTemplate, "apply", "-f", "-")

	By("setting up default NetworkPolicyAdmissionRules")
	kubectlSafe(bmcDenyNetworkPolicyAdmissionRule, "apply", "-f", "-")
	kubectlSafe(nodeDenyNetworkPolicyAdmissionRule, "apply", "-f", "-")
	kubectlSafe(excludeOnlyNetworkPolicyAdmissionRule, "apply", "-f", "-")
})
