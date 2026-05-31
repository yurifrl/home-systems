# Tailscale auth-key refresh in nostos

`tailscale://authkey` is the dynamic secret reference for Tailscale auth keys.
Use it where the Talos Tailscale extension expects `TS_AUTHKEY`:

```yaml
- TS_AUTHKEY=tailscale://authkey?description=tp1
```

On every render:

```bash
nostos render tp1
```

nostos resolves `tailscale://authkey` by minting a fresh Tailscale auth key
through the configured Tailscale OAuth backend. The rendered machineconfig gets
the real `tskey-auth-...` value. The source template keeps only the
`tailscale://authkey` reference.

This means Tailscale auth keys are refreshed on render, not stored statically in
Git.

## Query parameters

`description` labels the minted key in Tailscale:

```text
tailscale://authkey?description=tp1
```

Other optional query parameters override the defaults from `secrets.tailscale`
for that render:

```text
tailscale://authkey?tags=tag:k8s,tag:worker
tailscale://authkey?expiry=86400
tailscale://authkey?reusable=false
tailscale://authkey?ephemeral=false
tailscale://authkey?preauthorized=true
```

Unset values inherit from `nostos/config.yaml`:

```yaml
secrets:
  tailscale:
    tags: [tag:k8s]
    expiry: 7776000
    reusable: false
    ephemeral: false
    preauthorized: true
    description: nostos
```

## Why this exists

- No static Tailscale auth keys in git.
- Each render can mint a node-labelled key.
- Key defaults stay centralized in `nostos/config.yaml`.
- Short-lived, non-reusable keys reduce blast radius if a rendered config leaks.
