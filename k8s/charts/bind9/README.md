
# References
- [artem-shestakov/helm-charts](https://github.com/artem-shestakov/helm-charts/tree/master/charts/bind9)
- [johanneskastl/helm-charts](https://github.com/johanneskastl/helm-charts/tree/main/charts/bind9)
- [johanneskastl/bind9-isc-helm-chart](https://github.com/johanneskastl/bind9-isc-helm-chart/tree/main/charts/bind9)


# Bind9 external-dns issue

Troubleshoot:

```bash
k -n external-dns logs -l app.kubernetes.io/instance=rfc2136
k -n bind9 logs -l app=bind9

# Secrets
k -n external-dns get secrets tsig-secret -ojsonpath="{.data.rfc2136_tsig_secret}" | base64 -d
k -n bind9 get secrets bind9-keys -ojsonpath="{.data.keys\.conf}" | base64 -d

# Fails
nslookup ha.syscd.dev 192.168.68.200
# Server:         192.168.68.200
# Address:        192.168.68.200#53
# ** server can't find ha.syscd.dev: NXDOMAIN
```
    