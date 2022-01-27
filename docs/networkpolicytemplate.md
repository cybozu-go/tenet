# NetworkPolicyTemplate
`NetworkPolicyTemplate` enables administrators to write `CiliumNetworkPolicy` templates that tenants can opt-into via the `tenet.cybozu.io/network-policy-template` annotation in their `Namespace` resources. Templates can be supplied with values sources from the `.metadata` field of the `Namespace` resource that reference them. When annotations are placed on a root namespace managed by Accurate the annotations, and thus the templated CiliumNetworkPolicies, can be propagated to child namespaces. For instance, given the following `NetworkPolicyTemplate`,

```yaml
# network-policy-template.yaml
apiVersion: tenet.cybozu.io/v1beta1
kind: NetworkPolicyTemplate
metadata:
    name: allow-intra-namespace-egress
spec:
    policyTemplate: |
      apiVersion: cilium.io/v2
      kind: CiliumNetworkPolicy
      metadata:
        name: allow-intra-namespace-egress
      spec:
        endpointSelector: {}
        egress:
        - toEndpoints:
          - matchLabels:
              "k8s:io.kubernetes.pod.namespace": {{.Name}}
```

When a tenant namespace is annotated like below,

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
  annotations:
      tenet.cybozu.io/network-policy-template: allow-intra-namespace-egress
  labels:
      accurate.cybozu.com/type: root
```

The following `CiliumNetworkPolicy` gets created in the `my-namespace` namespace:

```yaml
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: allow-intra-namespace-egress
  namespace: my-namespace
spec:
  endpointSelector: {}
  egress:
  - toEndpoints:
    - matchLabels:
         "k8s:io.kubernetes.pod.namespace": my-namespace
```

If `my-namespace` is an Accurate root namespace, any of its child namespace will inherit the `tenet.cybozu.io/network-policy-template` annotation and CiliumNetworkPolicies will be created with the templates filled-in.
