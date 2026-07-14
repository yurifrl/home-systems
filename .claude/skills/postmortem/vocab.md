# Controlled vocabulary for postmortem frontmatter

Pick existing tags when they fit; append new ones when they don't. Tags are the
recurrence-detection grep key — free text here defeats the skill's purpose.

## components
argocd, cloudflared, cilium, etcd, coredns, longhorn, cnpg, crossplane,
metallb, istio, tailscale, gatus, kube-apiserver,
dell01, tp1, tp4, pc01, rpi01, macarm01, macintel01

## failure_mode
vxlan-tx-checksum-offload, cilium-stale-bpf-state, etcd-quorum-loss,
cross-family-vxlan-endpoint-mesh, tailscale-cilium-endpoint-recursion,
tailscale-accept-routes-lan-hijack, tailscale-config-flag-drift-crashloop,
conversion-webhook-blocks-argocd-sync, apiserver-crd-cache-oom,
cloudflared-tunnel-connector-gap, cloudflared-quic-edge-unreachable,
aggressive-liveness-probe, cross-node-tcp-broken, dns-timeout,
longhorn-sharemanager-on-cross-site-node
