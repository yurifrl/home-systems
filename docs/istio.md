
## Install

- [From Zero to Istio Ambient + Argo CD on KinD in 15 Minutes! | Solo.io](https://www.solo.io/blog/istio-ambient-argo-cd-kind-15-minutes)

## Troubleshoot connectivity issues with ztunnel
- [Istio / Troubleshoot connectivity issues with ztunnel](https://istio.io/latest/docs/ambient/usage/troubleshoot-ztunnel/)
- [Istio / Install the Istio CNI node agent](https://istio.io/latest/docs/setup/additional-setup/cni/)


```bash
# Test if requests are being routed through ztunnel
kubectl -n istio-system logs -l app=ztunnel -f | grep -E "inbound|outbound"
```
