# Home Systems


## Quickwit

- [Quickwit\_Otel](https://capten.ai/learning-center/5-learn-devops/quickwit/quickwit_with_otel/)
- [helm-charts/charts/quickwit/values.yaml at main · quickwit-oss/helm-charts](https://github.com/quickwit-oss/helm-charts/blob/main/charts/quickwit/values.yaml)
- [Logs and Traces with Grafana | Quickwit](https://quickwit.io/docs/get-started/tutorials/trace-analytics-with-grafana)



# DNS

apt-get update & apt-get install dnsutils
echo -e "server 192.168.68.200\nzone syscd.dev\nupdate add httpbin.syscd.dev 60 A 192.168.68.201\nsend" | nsupdate -v -k /etc/bind/keys.conf


Not sure if can help
[Installing Monkale CoreDNS Manager Operator on Single-Node Talos | by Nicholas | Medium](https://medium.com/@nikolay-udovik/installing-monkale-coredns-manager-operator-on-single-node-talos-16f8be900585)
[How to run a dns server on hostNetwork for external requests? · siderolabs/talos · Discussion #9921](https://github.com/siderolabs/talos/discussions/9921)
the are something on using local registry here [How to start Host DNS? · siderolabs/talos · Discussion #9434](https://github.com/siderolabs/talos/discussions/9434)