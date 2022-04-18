package cilium

// RuleType specifies lookup values for CIDR-based policies.
type RuleType struct {
	Type     string
	RuleKeys map[RuleKey]string
}

type RuleKey string

const (
	CIDRRuleKey    RuleKey = "cidr"
	CIDRSetRuleKey RuleKey = "cidrset"
	EntityRuleKey  RuleKey = "entity"
)

var (
	EgressRule = RuleType{
		Type: "egress",
		RuleKeys: map[RuleKey]string{
			CIDRRuleKey:    "toCIDR",
			CIDRSetRuleKey: "toCIDRSet",
			EntityRuleKey:  "toEntities",
		},
	}
	IngressRule = RuleType{
		Type: "ingress",
		RuleKeys: map[RuleKey]string{
			CIDRRuleKey:    "fromCIDR",
			CIDRSetRuleKey: "fromCIDRSet",
			EntityRuleKey:  "fromEntities",
		},
	}
)
