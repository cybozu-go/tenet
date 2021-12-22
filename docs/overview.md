# Overview
Tenet is a Kubernetes controller that aims to facilitate setting-up Network Policies on tenant namespaces.

It is designed to work in conjunction with [Cilium Network Policies](https://docs.cilium.io/en/stable/policy/) and [Accurate](https://cybozu-go.github.io/accurate/).

## Motivation
To enhance the security without sacrificing convenience, we want to provide default network policies for tenants to consume. For instance, we want egress communications to be disabled by default in tenant namespaces. Tenants should be able to add exceptions to fit the needs of their workflows while being prevented from adding exceptions that are too broad, i.e. grant access to `node` resources.

## Features
- Allow cluster administrators to provide network policy templates tenants can opt into
  - currently only `CiliumNetworkPolicy` templates are supported
- Automatically generate network policies on namespaces that opt into them
  - when used in conjunction with `Accurate`, resource generation is also performed on SubNamespaces
- Allow cluster administrators to place restrictions on the expressivity of network policies
  - currently only IP address restrictions are supported
