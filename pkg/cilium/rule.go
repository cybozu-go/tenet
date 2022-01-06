package cilium

// RuleType specifies lookup values for CIDR-based policies.
type RuleType struct {
	Type       string
	CIDRKey    string
	CIDRSetKey string
}

var (
	EgressRule = RuleType{
		Type:       "egress",
		CIDRKey:    "toCIDR",
		CIDRSetKey: "toCIDRSet",
	}
	IngressRule = RuleType{
		Type:       "ingress",
		CIDRKey:    "fromCIDR",
		CIDRSetKey: "fromCIDRSet",
	}
)
