# ax — remote agent execution

ax runs agent tasks on cluster workers instead of your laptop. Control plane is
`ax-server` (gRPC, ns `ax-system`); the `ax` CLI talks to it. Runtime secrets live
in ExternalSecrets (`gemini` key); workloads deploy through ArgoCD:

- `k8s/applications/ax.yaml` — app (chart `k8s/charts/ax` + support chart VS)
- `k8s/applications/istio-gateway.yaml` — LB ports (incl. the ax CLI listener)
- `k8s/charts/support-cluster/templates/gateways.yaml` — gateway servers

## Access

The CLI only speaks **plaintext gRPC** — it strips `https://` and dials
insecure (`github.com/google/ax` `cmd/ax/main.go getAXClient`). Two working
paths:

| Path | `AX_SERVER` | Who |
|---|---|---|
| Gateway h2c (default) | `http://ax.syscd.tech:18080` | machines in the tailnet |
| kubectl tunnel | unset — auto-connects | machines outside the tailnet |

Server resolution order: `--server` flag → `$AX_SERVER` → tunnel → `http://localhost:8080`.

```bash
# Tailnet machine (recommended)
export AX_SERVER=http://ax.syscd.tech:18080

# Outside the tailnet — CLI spawns a port-forward on first command
ax get tasks
```

- Port 18080 is plaintext h2c, routed only for `ax.syscd.tech` on the
  tailscale gateway — never on the public cloudflare domains (the API has no
  auth).
- `https://ax.syscd.tech:443` works for `curl`, but **not** for the CLI
  (insecure client + TLS listener → `error reading server preface:
  connection reset by peer`).
- A stale tunnel shows in `ax tunnel list` as `Stale / Not responding`;
  clear it with `ax tunnel stop`.

## Commands

```bash
export AX_SERVER=http://ax.syscd.tech:18080

ax get tasks                 # list tasks
ax get task <name>           # full task spec
ax get workspaces            # list workspaces
ax get workspace <name>      # workspace detail (git repos, mcp servers)
ax get models                # list models
ax get model <name>
ax version
```

Task lifecycle:

```bash
ax apply -f task.yaml        # create from manifest (or -f - for stdin)
ax resume <name>             # resume a suspended task
ax ssh <name>                # shell into the task's worker
ax delete <name>             # delete a task
```

## Example: define a task

```yaml
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: demo
  atespace: default          # atespace scope (ax -a flag default: "default")
spec:
  image: ghcr.io/yurifrl/omp-remote-agents:latest
  command:
    - /bin/sh
    - -c
    - |
      echo hello from a cluster worker
  debug: true
```

```bash
ax apply -f task.yaml
ax get task demo
ax ssh demo
ax delete demo
```

Workspaces are separate objects (`ax get workspaces`) that register git repos;
models select the LLM provider (`gemini-flash` → google, key from ExternalSecret
`gemini`). See `ax get task <name>` output for how an existing task references them.

## Troubleshooting

- **`error reading server preface ... connection reset by peer`** — the CLI
  dialed TLS. Set `AX_SERVER=http://ax.syscd.tech:18080` (scheme `http://`,
  port `18080`).
- **CLI hangs / no tunnel, no server** — auto-detect failed: check
  `kubectl config current-context` is `admin@talos-default`, or set
  `AX_SERVER` explicitly.
- **After chart changes** — nothing syncs by hand: commit + push, wait for
  ArgoCD (`kubectl get application ax -n argocd`), verify with
  `AX_SERVER=... ax get tasks`. Never `kubectl apply`.
- **Istio protocol** — ax-server's Service port must be named `grpc`
  (h2c upstream); the gateway plaintext listener is `protocol: HTTP2`. If
  gRPC through the gateway breaks, check
  `istioctl pc cluster <gateway-pod> -n istio-system | grep ax` shows the
  `-h2` cluster.
