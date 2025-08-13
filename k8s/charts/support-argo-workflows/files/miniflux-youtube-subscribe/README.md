Miniflux YouTube Subscribe

Files in this directory are embedded by the Helm chart and can also be run locally.

Contents
- channels.json: JSON list of channels to subscribe (name + id)
- subscribe.py: Script that reads channels JSON from stdin and calls Miniflux API

Local run
```bash
export MINIFLUX_URL=https://miniflux.syscd.tech
export MINIFLUX_TOKEN=<token>
export MINIFLUX_CATEGORY_ID=1 # optional; defaults to 1

cat k8s/charts/support-argo-workflows/files/miniflux-youtube-subscribe/channels.json \
  | python3 k8s/charts/support-argo-workflows/files/miniflux-youtube-subscribe/subscribe.py
```
