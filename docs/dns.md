# DNS

apt-get update & apt-get install dnsutils
echo -e "server 192.168.68.200\nzone syscd.dev\nupdate add httpbin.syscd.dev 60 A 192.168.68.201\nsend" | nsupdate -v -k /etc/bind/keys.conf


Not sure if can help
- [Installing Monkale CoreDNS Manager Operator on Single-Node Talos | by Nicholas | Medium](https://medium.com/@nikolay-udovik/installing-monkale-coredns-manager-operator-on-single-node-talos-16f8be900585)
- [How to run a dns server on hostNetwork for external requests? · siderolabs/talos · Discussion #9921](https://github.com/siderolabs/talos/discussions/9921)
- the are something on using local registry here [How to start Host DNS? · siderolabs/talos · Discussion #9434](https://github.com/siderolabs/talos/discussions/9434)

