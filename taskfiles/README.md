# Using nostos 

Nostos is the canonical way to provision and manage Talos nodes in this cluster. It's a wrapper around `talosctl`

> **Rule:** Nostos owns Talos provisioning and lifecycle. If you're about to
> run a raw `talosctl` command for provisioning, render, bootstrap, install,
> kubeconfig, or node management — stop, nostos already does it.

---

## Setup

Use exactly one kubeconfig flow for this repo: **nostos owns kubeconfig**.

```bash
nostos kubeconfig
kubectl config use-context admin@talos-default
```

`nostos kubeconfig` writes the kubeconfig to `~/.talos/kubeconfig`. The repo
`.mise.toml` sets `KUBECONFIG` to that file automatically.

---

## Taskfile commands → nostos commands

This is the migration guide for replacing old taskfile habits with direct
nostos commands. It is organized by intent instead of as a wide table so it is
readable in a terminal.

### Provisioning and install lifecycle

These are the commands nostos should replace.

#### `task talos:apply`

Deprecated wrapper that already exits and tells you to use nostos.

Use instead:

```bash
nostos node install <node>
```

Status: **replace with nostos**.

Notes:

- Do not restore a `task nostos:*` wrapper.
- Call nostos directly.

#### `task talostroobleshooting:apply`

Applies rendered Talos configs directly to rpi, tp4, tp1, and vm-pc01 with
`talosctl apply-config`.

Use instead:

```bash
nostos node install <node>
```

Status: **replace with nostos**.

Notes:

- For provisioning and install lifecycle, nostos owns this path.
- Use direct `talosctl apply-config` only for exceptional manual repair.

#### `task talostroobleshooting:apply-controlplane`

Applies the rpi controlplane config with `talosctl apply-config`.

Use instead:

```bash
nostos node install <controlplane-node>
```

Status: **replace with nostos**.

Notes:

- Use the node name from `nostos/config.yaml`.
- If the controlplane is already installed and only needs etcd bootstrap, use
  `nostos bootstrap <node>`.

#### `task talos:pc01-install`
#### `task talostroobleshooting:pc01-install`

Downloads a Talos raw disk image, writes it to `/dev/disk5` with `dd`, then
applies the pc01 config.

Use instead when the node can use a nostos provisioner:

```bash
nostos node install vm-pc01
```

Status: **partial**.

Notes:

- Nostos install works when the node's configured boot method supports it.
- The USB `dd` image flashing flow remains manual if PXE/TPI cannot be used.

#### `task turing:flash`
#### `task turing:install-talos`

Deprecated Turing Pi install wrappers that point to nostos.

Use instead:

```bash
nostos node install <node>
```

Status: **replace with nostos**.

Notes:

- These were retired because they bypassed the secrets pipeline.
- Nostos supports method-dispatched installs, including TPI when configured.

#### `task turing:download`

Deprecated image download task.

Use instead:

```bash
nostos build
nostos node install <node>
```

Status: **replace with nostos**.

Notes:

- Nostos owns the image cache/build flow and install flow.

### Secret injection and rendered configs

#### `task talos:op:inject`

Runs `op inject` for worker Talos YAML files into `talos/op/nodes/`.

Use instead:

```bash
nostos render <node>
```

Status: **replace with nostos**.

Notes:

- Run once per node.
- Nostos renders from the canonical templates and resolves secrets through the
  configured secret backend.

#### `task talostroobleshooting:onepassword`

Runs `op inject` for rpi, workers, and talosconfig into `talos/op/`.

Use instead for node machineconfigs:

```bash
nostos render <node>
```

Status: **partial**.

Notes:

- Node machineconfigs move to `nostos render <node>`.
- Talosconfig injection does not fully move until `nostos config refresh` works.

### Kubeconfig and talosconfig

#### Kubeconfig setup

Use nostos:

```bash
nostos kubeconfig
kubectl config use-context admin@talos-default
```

Status: **replace with nostos**.

Notes:

- Nostos writes kubeconfig to `~/.talos/kubeconfig`.
- `.mise.toml` sets `KUBECONFIG` to that file automatically.
- Agents and humans should use that same file.

#### Talos client config

Talos client/admin config is separate from kubeconfig.

Possible future nostos command:

```bash
nostos config refresh
```

Status: **partial**.

Notes:

- `nostos config refresh` exists, but the CLI marks admin-cert refresh as not
  fully implemented.
- Keep manual talosconfig handling only for Talos client cert/admin config until
  refresh is implemented and verified.

### Dashboards and status

#### `task talos:dashboard`
#### `task talostroobleshooting:dashboard`

Opens `talosctl dashboard` across all known nodes.

Use instead:

```bash
nostos dashboard
```

Status: **replace with nostos**.

Notes:

- Same high-level purpose.
- Nostos is the canonical single-pane TUI for cluster, nodes, and ArgoCD apps.

### Upgrades

Nostos does **not** replace these.

#### `task talostroobleshooting:upgrade-1.10.3-tailscale`
#### `task talosupgrade-1.10.3:upgrade-1.10.3-tailscale`

Runs `talosctl upgrade` on pc01, tp1, tp4, and rpi with node-specific factory
images for v1.10.3.

Nostos equivalent: **none**.

Status: **keep manual**.

Notes:

- Nostos has no upgrade command.
- Do not invent one.
- Keep this as manual `talosctl upgrade` documentation or as a task.

#### `task talostroobleshooting:upgrade-1.9.4`
#### `task talosupgrade-1.10.3:upgrade-1.9.4`

Runs `talosctl upgrade` on all nodes with v1.9.4 factory images.

Nostos equivalent: **none**.

Status: **keep manual**.

Notes:

- Nostos has no upgrade command.
- Keep version/image-specific upgrade state outside nostos.

### Diagnostics and manual repair

Nostos does **not** replace raw diagnostics.

#### `task talostroobleshooting:tailscale-status`

Reads extension configs, service state, and recent Tailscale logs from every
node.

Nostos equivalent: **none**.

Status: **keep manual**.

Use direct Talos commands:

```bash
talosctl get extensionserviceconfigs
talosctl service ext-tailscale
talosctl logs ext-tailscale --tail=10
```

#### `task talostroobleshooting:other`

Mixed scratch Talos commands: patch machineconfig, get extension configs, dmesg.

Nostos equivalent: **none**.

Status: **keep manual**.

Notes:

- This is troubleshooting/scratch work, not nostos lifecycle.
- Prefer direct `talosctl` commands so the intent is explicit.

#### `task turing:get`

Deprecated wrapper that points users to `tpi uart get -n <slot>` directly.

Nostos equivalent: **none**.

Status: **keep manual**.

Notes:

- UART read is board diagnostics, not nostos provisioning.

### Not related to nostos

These taskfiles are separate concerns. Do not migrate them to nostos.

- `task argo:repo-add`: add/update the Argo CD Helm repo.
- `task argo:update`: render Argo CD manifests with Helm.
- `task argo:apply`: apply rendered Argo CD manifests with `kubectl`.
- `task kubernetes:1password`: create 1Password Connect resources/secrets for
  external-secrets.
- `task proxmox:copy`: sync Proxmox VM config files to the Proxmox host.

### Quick decision rules

- Provisioning, render, bootstrap, install, kubeconfig, node lifecycle: use
  **nostos**.
- Kubeconfig setup has exactly one path: **`nostos kubeconfig`**, then use
  `~/.talos/kubeconfig`.
- Talos upgrades: keep **manual `talosctl upgrade`** tasks/docs.
- Raw diagnostics and repair commands: use **direct `talosctl`**.
- ArgoCD, Kubernetes app setup, Proxmox: **not nostos**.

### Useful nostos command reference

```bash
nostos build                         # download Talos assets + build iPXE assets
nostos render <node>                 # render one node machineconfig with secrets
nostos pxe --iface <iface>           # serve PXE assets
nostos node install <node> --yes     # end-to-end install
nostos bootstrap <node>              # bootstrap first controlplane
nostos kubeconfig                 # canonical kubeconfig setup; writes ~/.talos/kubeconfig
nostos status --output json          # machine-readable node reachability/version
nostos dashboard                     # interactive cluster dashboard
nostos node list --output json       # machine-readable node registry/status
nostos node show <node>              # one node reachability/config
nostos wipe <node>                   # queue one-shot wipe on next PXE boot
nostos nuke --yes                    # remove regenerable nostos state
nostos cluster cleanup --dry-run     # preview k8s/Tailscale zombie cleanup
```

---

## Commands

### Provisioning a node (end to end)

```bash
nostos build                      # download Talos assets + build iPXE binary
nostos render dell01              # render dell01's machineconfig, secrets injected
nostos pxe --iface en5            # start PXE server (HTTP + dnsmasq); needs sudo
nostos node install dell01        # full install (PXE). add -- --reinstall --yes to redo
nostos bootstrap dell01           # bootstrap etcd on first controlplane only
nostos kubeconfig                 # canonical kubeconfig setup; writes ~/.talos/kubeconfig
```

### Day-to-day

```bash
nostos status                     # per-node reachability + Talos version
nostos dashboard                  # live TUI: cluster + nodes + ArgoCD apps
nostos node list                  # registered nodes with live reachability
nostos node show dell01           # one node's reachability + config
```

### Node registry

```bash
nostos node add <name>            # interactively register a new node
nostos node remove <name>         # remove from config.yaml
```

### Maintenance

```bash
nostos wipe dell01                # queue one-shot disk wipe on next PXE boot
nostos cluster cleanup            # reconcile k8s + Tailscale state; remove zombies
nostos nuke                       # delete state/ (safe; regenerable)
nostos secrets list               # configured secret backends + Validate() status
nostos secrets test               # validate backends; mints+revokes a tailscale smoke key
```

### Machine-readable output

Every command takes `--output json` for scripting/agents:

```bash
nostos status --output json
```

---

## What nostos does NOT do

Nostos is scoped to provisioning + lifecycle. For these, use `talosctl` directly:

1. **Talos version upgrades.** There is no `nostos upgrade`. Upgrade by hand:
   ```bash
   talosctl -n <node-ip> upgrade --image factory.talos.dev/metal-installer/<id>:<ver> --wait=false
   ```
   See `../docs/talos.md` for the image IDs per node.
2. **Raw diagnostics.** `talosctl logs`, `dmesg`, `get extensionserviceconfigs`,
   `service`, Tailscale status — run `talosctl` directly. Nostos doesn't wrap reads.
3. **USB image flashing.** Nodes that can't PXE boot still need the `dd` flow.
   Not a nostos workflow.

Also: `nostos config refresh` (admin-cert regen) is **declared but not yet
implemented**. Talosconfig handling stays manual until it lands.

---

## Rules for AI agents

- Provisioning, render, bootstrap, install, kubeconfig, node management → **nostos**, called directly.
- Upgrades and raw diagnostics → `talosctl` directly. Do NOT invent a nostos command; those don't exist.
- `~/.local/share/nostos/` holds regenerable nostos runtime cache: assets, rendered machineconfigs, locks, logs, and digests.

---

## Other taskfiles in this directory

These are separate concerns, not Talos provisioning:

| File | Scope |
|------|-------|
| `argo.yml` | ArgoCD operations |
| `kubernetes.yml` | Kubernetes secret setup |
| `proxmox.yml` | Proxmox VMs |
| `turing.yml` | Turing Pi board ops |
| `talos.yml`, `talostroobleshooting.yml`, `talosupgrade-1.10.3.yml` | Legacy manual `talosctl` ops — prefer nostos for provisioning; keep only for upgrades/diagnostics nostos can't do |

---

## See also

- `../nostos/README.md` — data layout (config, templates, state)
- `../.submodules/nostos/README.md` — full CLI reference
- `../docs/talos.md` — manual Talos notes (upgrades, troubleshooting)
