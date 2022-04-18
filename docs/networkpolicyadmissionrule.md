# NetworkPolicyAdmissionRule
To restrict the scope of whitelist permissions tenants can write via CiliumNetworkPolicies or NetworkPolicies, cluster administrators can write `NetworkPolicyAdmissionRule` resources. This allows administrators to forbid the use of specific CIDR ranges as endpoint selectors for network policies. For instance, the following `NetworkPolicyAdmissionRule` will reject network policies in namespaces that do not hold the team: neco label, i.e. all tenant namespaces, from specifing IP addresses within the 10.72.16.0/20 range for egress rules.

```yaml
# admission-rule.yaml
apiVersion: tenet.cybozu.io/v1beta1
kind: NetworkPolicyAdmissionRule
metadata:
    name: forbid-bmc
spec:
    namespaceSelector:
      excludeLabels:
        team: neco
    forbiddenIPRanges:
      - cidr: 10.72.16.0/20
        type: egress
```

IP address restrictions can be applied on ingress or egress type network policies. When `type: all` is specified, the restrictions apply to both ingress and egress.

## Specifications

### namespaceSelector

This selects namespaces for which the admission rules apply.

### forbiddenIPRanges

This defines IP ranges, in CIDR form, against which users cannot define network policies.

### forbiddenEntities

This defines Cilium entities that users are not allowed to refer to in their network policies.
