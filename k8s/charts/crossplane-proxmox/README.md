# crossplane-proxmox

One self-contained chart that provisions and operates the VMs on the `pc01`
Proxmox host **declaratively and self-healingly** — no laptop scripts, no
`hack/`, no `Dockerfile`. Everything is a Crossplane resource or a native
Kubernetes Job/CronJob, deployed by ArgoCD.

```
ArgoCD ─(PreSync)→ preflight Job ──(fail-fast if deps missing)
       └─ Crossplane: ProviderConfig → EnvironmentDownloadFile(s) → EnvironmentVM(s)
                                  │                                   │
       CronJob (iso-pipeline): build-if-missing → upload → sign → patch URL
                                  ▼                                   ▼
                         GCS (private bucket)                Proxmox host pc01
                                                          VM 100 talos · VM 101 windows
```

## What it manages

| Resource | Purpose |
|----------|---------|
| `ProviderConfig` + `ExternalSecret` | connection to the Proxmox API (creds from 1Password) |
| `EnvironmentDownloadFile` ×N | Proxmox pulls each ISO onto `local:iso` (declarative) |
| `EnvironmentVM` talos-pc01 / windows | the two VMs, 1:1 with `proxmox/100.conf` / `101.conf` |
| `CronJob` iso-pipeline | builds/uploads the Win11 combined ISO if missing, re-signs the URL weekly, patches the download MR — **self-heals** the two manual gaps |
| `Job` preflight (PreSync hook) | **fails the sync** if the provider CRDs or creds Secret are missing |

## Install

```bash
kubectl apply -f k8s/applications/crossplane-proxmox.yaml   # app-of-apps style
```
ArgoCD merges the public chart with the **private** values overlay
(`home-systems-values/proxmox/values.yaml`) for the bits that must not be public
(GCS bucket/project, `isoPipeline.enabled`, `windows.enabled`).

Prerequisites (checked by preflight, fail-fast):
- `crossplane-providers` deployed (the `provider-proxmox-bpg` CRDs)
- 1Password items: `crossplane-proxmox` (API creds), `crossplane-gcp` (GCS key),
  `windows-pc01-admin` (Windows admin password), reachable via the `onepassword`
  `ClusterSecretStore`.

## Operating it

**Switch the shared GPU (`0000:01:00.0`) between the VMs** — it's a one-line git
change, no script:
- Talos owns the GPU: `vms.talosPc01.started: true`, `vms.windows.started: false`.
- Windows owns the GPU (production): `vms.windows.started: true`,
  `vms.windows.passthrough: true`, and `vms.talosPc01.started: false`.

The chart **fails to render** (`templates/00-guards.yaml`) if both VMs are
started while both claim the GPU — the mutual-exclusion invariant is enforced in
the template, not by hope.

**Windows install vs production variant** (`vms.windows`):
- install: `vga: std`, `passthrough: false` — observable, no GPU contention.
- production: `passthrough: true`, `vga: null` — GPU passthrough, no VNC.

**Rebuild the Win11 ISO now** (instead of waiting for the weekly cron):
```bash
kubectl -n crossplane-system create job --from=cronjob/crossplane-proxmox-iso-pipeline iso-rebuild
# force a fresh build even if the object exists: set env FORCE_REBUILD=1 on the job
```

## Self-healing properties

- **Host wiped / VM deleted** → ArgoCD + Crossplane recreate the VMs and re-pull
  ISOs from git. Talos auto-installs via nostos (`nostos/templates/talos-pc01.yaml`).
- **Signed URL near expiry** → the weekly CronJob re-signs (TTL ≤ 7d) and patches
  the download MR; ArgoCD `ignoreDifferences` lets the cron own that URL field.
- **Bucket object missing** → the CronJob rebuilds and re-uploads it (nixery
  toolchain, `7z` extract + `xorriso` repack, no privileged, no Dockerfile).
- **Missing dependency** → preflight fails the sync with a clear message rather
  than leaving broken resources.

## Files

```
Chart.yaml  values.yaml  values.schema.json  README.md
files/autounattend.xml              # baked into the combined ISO (pw is a build-time placeholder)
templates/
  00-guards.yaml                    # render-time GPU-conflict guard
  preflight.yaml                    # PreSync fail-fast Job
  providerconfig.yaml               # ProviderConfig + creds ExternalSecret
  downloads.yaml                    # EnvironmentDownloadFile per ISO
  talos-pc01.yaml  windows.yaml     # the two EnvironmentVMs
  iso-pipeline.yaml                 # CronJob + ConfigMap(script+autounattend) + RBAC
  iso-pipeline-secrets.yaml         # GCS key + win-admin ExternalSecrets
```

The Talos OS layer for VM 100 lives with the rest of the Talos nodes in
`nostos/` (`nostos/config.yaml` + `nostos/templates/talos-pc01.yaml`), applied
by `nostos`, not this chart.
