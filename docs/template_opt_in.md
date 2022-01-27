# Template Opt-in
Tenants and users can opt into network policy templates via the following annotation placed on `Namespace` resources:

- `tenet.cybozu.io/network-policy-template` - a comma-separated list of `NetworkPolicyTemplate` names

In a cluster where cluster administrators have control over `Namespace` definitions, for instance in a situation where [Accurate](https://cybozu-go.github.io/accurate/) is deployed and cluster administrators manage root namespaces, users will not be able to remove inherited annotations to bypass restrictions.

## Example
```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
  annotations:
      tenet.cybozu.io/network-policy-template: allow-intra-namespace-egress,forbid-bmc
  labels:
      accurate.cybozu.com/type: root
```

This will create the appropriate CiliumNetworkPolicies as defined in the relevant NetworkPolicyTemplates.
