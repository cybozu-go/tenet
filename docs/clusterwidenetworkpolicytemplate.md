# ClusterwideNetworkPolicyTemplate
`ClusterwideNetworkPolicyTemplate` works similarly to `NetworkPolicyTemplate` but serves to write `CiliumClusterwideNetworkPolicy` templates that tenants can opt-into via the `tenet.cybozu.io/network-policy-template` annotation in their `Namespace` resources. Unlike CiliumNetworkPolicies created from a `NetworkPolicyTemplate`, CiliumClusterwideNetworkPolicies created from `ClusterwideNetworkPolicyTemplate` are cluster-wide resources.

```yaml
# clusterwide-network-policy-template.yaml
apiVersion: tenet.cybozu.io/v1beta1
kind: ClusterwideNetworkPolicyTemplate
metadata:
  name: allow-team-ingress
spec:
  policyTemplate: |
    apiVersion: cilium.io/v2
    kind: CiliumClusterwideNetworkPolicy
    metadata:
      name: {{.Name}}-allow-team-ingress
    spec:
      endpointSelector:
        matchLabels:
          k8s:io.kubernetes.pod.namespace: {{.Name}}
      ingress:
        - fromEndpoints:
          - matchLabels:
              "k8s:io.cilium.k8s.namespace.labels.team": {{ index .Labels "team" }}
```

When a tenant namespace is annotated like below,

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
  annotations:
      tenet.cybozu.io/network-policy-template: allow-team-ingress
  labels:
      accurate.cybozu.com/type: root
      team: my-team
```

The following `CiliumClusterwideNetworkPolicy` will be created with cluster scope:

```yaml
apiVersion: cilium.io/v2
kind: CiliumClusterwideNetworkPolicy
metadata:
  name: my-namespace-allow-team-ingress
spec:
  endpointSelector:
    matchLabels:
      k8s:io.kubernetes.pod.namespace: my-namespace
  ingress:
    - fromEndpoints:
      - matchLabels:
          "k8s:io.cilium.k8s.namespace.labels.team": my-team
```
