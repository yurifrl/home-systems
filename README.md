# Home Systems


## Quickwit

- [Quickwit\_Otel](https://capten.ai/learning-center/5-learn-devops/quickwit/quickwit_with_otel/)
- [helm-charts/charts/quickwit/values.yaml at main · quickwit-oss/helm-charts](https://github.com/quickwit-oss/helm-charts/blob/main/charts/quickwit/values.yaml)
- [Logs and Traces with Grafana | Quickwit](https://quickwit.io/docs/get-started/tutorials/trace-analytics-with-grafana)



# Dbs test

apt-get update & apt-get install dnsutils
echo -e "server 192.168.68.200\nzone syscd.dev\nupdate add httpbin.syscd.dev 60 A 192.168.68.201\nsend" | nsupdate -v -k /etc/bind/keys.conf