# Changelog

## 2026-06-06 Hermes hctl CLI, Generic Repo Sync, Obsidian Removal
- Session ID: 019e9578-e332-7ace-8235-26deeb545c82
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-06-05T01-49-48-210Z_019e9578-e332-7ace-8235-26deeb545c82.jsonl
- Session Name: 2026-06-06-1137-flash-command-deployment
- Context Name: 2026-06-06-1137-flash-command-deployment

### Added
- `k8s/images/hermes/hctl`: single-file Python CLI baked into the hermes image, stdlib-only. Subcommands: `auth login` (gh + git auth from `$GH_TOKEN`/`$GITHUB_TOKEN`), `op files sync` (1Password Connect REST attachments; supports `--connect-credentials-secret NS/NAME[:KEY]` to fetch the token via `kubectl get secret`), `repos clone --user U --repos a,b --dest /workdir`, `repos sync --dir /workdir --interval 1h` (long-running loop with SIGTERM/SIGINT trap that runs one final commit+push pass before exit; non-zero exit on push failure to surface in pod status).
- `k8s/charts/hermes/templates/_helpers.tpl`: `hermes-agent.workdirVolumeName` helper.
- Chart-managed init containers + sync sidecar in `k8s/charts/hermes/templates/statefulset.yaml`: `git-login`, `op-files`, `repos-clone` init containers and the `repos-sync` sidecar, all invoking `hctl`. `/workdir` is an inline `emptyDir` mounted on both `hermes-agent` and `repos-sync`.
- Structured chart values (default-disabled to keep the chart portable): `auth.gitLogin.enabled`, `opFiles.{enabled,host,vault,item,dest,connectCredentialsSecret}`, `gitRepos.{enabled,user,repos,mountPath,syncInterval}`.
- Session checkpoint context at `.agents/contexts/2026-06-06-1137-flash-command-deployment.md`.

### Changed
- `k8s/applications/hermes.yaml` collapsed from 300+ lines to ~60: only `virtualService.hosts`, `nodeSelector`, `tolerations`, and `auth/opFiles/gitRepos` feature flags. Private `gitRepos.user/repos` are merged from `$values/hermes/values.yaml` in `home-systems-values`.
- `k8s/images/hermes/Dockerfile`: drops the `op-files-sync` COPY; copies `hctl` to `/usr/local/bin/hctl` and chmods 0755.
- `k8s/charts/hermes/values.yaml`: removed Obsidian env (`OBSIDIAN_PATH`), Obsidian extra volume + mount, and the `hermes-obsidian` PVC; cleared the inline `extraInitContainers` git-login + op-files blobs (now rendered by the chart from structured values).

### Removed
- `k8s/images/hermes/op-files-sync` (replaced by `hctl op files sync`; moved to the normalized trash path under `/tmp/agents/removed/`).
- All Obsidian-specific configuration from the Hermes chart (env, volume mount, dedicated PVC).

## 2026-06-06 Remote Pi Onboarding, Nostos Flash Command, Cluster DNS Recovery
- Session ID: 019e92b3-0b8b-77c2-aed5-e1427247e838
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-06-04T12-54-27-979Z_019e92b3-0b8b-77c2-aed5-e1427247e838.jsonl
- Session Name: remote pi
- Context Name: remote pi

### Added
- `nostos flash` command (`internal/cli/flash.go`, `internal/cli/flash_test.go`): produces a flashable Talos disk image for any node — downloads raw image, mints fresh Tailscale auth key, renders machineconfig, writes to `--out FILE` (optional `--compress`) or `--device /dev/diskN` (gated on `--yes`). Full `--dry-run` plan envelope. Registered in cobra root and the `nostos schema` registry.
- `internal/image/` package (`builder.go`, `eeprom.go`, `builder_test.go`): `Builder` struct decompresses .raw.xz with `ulikunitz/xz`, writes to file (xz-compressed optional) or block device, emits a sidecar machineconfig, and for RPi nodes (`overlay: rpi_generic`) also emits an EEPROM recovery directory with `start4.elf`, `fixup4.dat`, `recovery.bin`, `pieeprom.bin`, `boot.conf` (`BOOT_ORDER=0xf21`).
- Multi-arch build in `internal/pxe/build.go`: `CollectAssetSpecs` walks every node in config, dedupes by (schematic, arch); `BuildAllNodes`, `DownloadAssetsForSpec`, `DownloadRPiFirmware`, `DownloadTalosRawImage` cache assets per spec; RPi nodes pull `start4.elf` + `fixup4.dat` from `raspberrypi/firmware`.
- `internal/config/config.go`: `Node.Overlay` (rpi_generic|turing_rk1) and `Node.Serial` fields added with validators.
- `nostos/templates/rpi01.yaml`: full machineconfig template for the offsite Pi 4 (controlplane, arm64, eth0 at 192.168.0.170, Tailscale extension with `--accept-routes`).
- `nostos/config.yaml`: `rpi01` node entry (MAC `e4:5f:01:3c:68:fa`, arm64 schematic `d0e797e7…`, overlay `rpi_generic`, install_disk `/dev/sda`).
- OpenSpec change `openspec/changes/nostos-flash-command/`: full proposal, design, three capability specs (image-build, zero-touch-enroll, multi-arch-assets), 33 implementation tasks (32 done; only the manual hardware e2e is outstanding).
- `docs/remote-node.md`: zero-touch enrollment guide covering rpi01-style offsite onboarding.

### Changed
- `internal/pxe/serve.go`: removed the `192.168.68.x` hardcoding from `detectNetwork` / `ipForInterface`; PXE server now auto-detects any RFC1918 private interface and derives the dnsmasq DHCP range + gateway from it (`inferDefaults`).
- `nostos build` (`internal/cli/commands.go`): default invocation is now multi-arch — walks every node and reports per-spec progress. `--arch` and `--legacy` preserve the v0.1 single-arch behaviour.
- `internal/registry/registry.go`: `Render` now warns to stderr when a rendered template has the Tailscale extension but no `--accept-routes` (warns by default; was the cross-subnet routing pitfall before — kept as a hint, not an error).
- All home-node templates have `TS_EXTRA_ARGS=--accept-routes` defaulted on (`tp1.yaml`, `tp4.yaml`, `dell01.yaml`, `rpi01.yaml`). NOTE: the dell01 default was reverted later in the session — see Removed.
- `nostos/templates/dell01.yaml`: pinned `machine.kubelet.nodeIP.validSubnets: [192.168.68.0/24]` so kubelet always picks the LAN IP as InternalIP (Talos was auto-selecting the Tailscale CGNAT IP, which broke kube-proxy Service-VIP DNAT). Added `cluster.allowSchedulingOnControlPlanes: true` so dell01 can host workloads while workers are out.
- `k8s/applications/istio-{base,cni,gateway,istiod,ztunnel}.yaml`: `targetRevision` rolled back from `1.30.0` to `1.26.2` to match the running istiod pod (chart 1.30 ConfigMaps use `omitNil` template func that 1.26 istiod can't parse, causing CrashLoopBackOff).
- `.submodules/nostos/AGENTS.md`: documented `flash` command invariants in the idempotency table; noted the per-invocation Tailscale-key minting cost.
- `.submodules/nostos/README.md` + `nostos/README.md`: `flash` quickstart, rpi01 entry, multi-arch build doc.

### Removed
- `TS_EXTRA_ARGS=--accept-routes` from `nostos/templates/dell01.yaml`: enabling it had imported `10.244.0.0/16 → tailscale0` (cluster pod CIDR) into dell01's routing table from advertised peer routes, breaking pod return-traffic asymmetrically and crashlooping CoreDNS for 25h+ (268 restarts). Architectural rule established: never `--accept-routes` on a node hosting cluster pods. Cross-LAN reach should use Tailscale CGNAT (`100.x.x.x`) only.
## 2026-06-05 Camofox Standalone, Self-Hosted Firecrawl, Browser Stack Trim
- Session ID: 019e8ae4-7c3c-7c96-aeb0-74aa13625541
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-06-03T00-31-30-364Z_019e8ae4-7c3c-7c96-aeb0-74aa13625541.jsonl
- Session Name: hermes-browser-backends-trim
- Context Name: undefined

### Added
- `k8s/applications/camofox.yaml`: standalone ArgoCD Application running `ghcr.io/jo-inc/camofox-browser:latest` via the support chart. ClusterIP service on 9377; 1 GiB memory-backed `/dev/shm`; pinned to dell01 with control-plane toleration; `/health` liveness/readiness probes.
- `k8s/applications/firecrawl.yaml`: ArgoCD Application that points at the local fork at `k8s/charts/firecrawl/` (was briefly upstream `firecrawl/firecrawl/examples/kubernetes/firecrawl-helm`, switched after we needed `nodeSelector` patches that upstream chart didn't expose). Sets `fullnameOverride: firecrawl`, pins all 9 deployments to dell01, drops `nuqWorker.replicaCount` to 1, keeps `nuqPrefetchWorker` enabled (it's the dispatcher), disables `extractWorker`, and overrides per-service `resources` for hobby sizing.
- `k8s/charts/firecrawl/`: forked from upstream commit `42b46be4f75a` (recorded in `.upstream-sha`). Single divergence from upstream: a `global.{nodeSelector,tolerations}` block plus per-component `resources` blocks added to all deployment templates (`deployment.yaml`, `worker-deployment.yaml`, `nuq-worker-deployment.yaml`, `nuq-prefetch-worker-deployment.yaml`, `extract-worker-deployment.yaml`, `playwright-deployment.yaml`, `rabbitmq-deployment.yaml`, `redis-deployment.yaml`, `nuq-postgres-deployment.yaml`). Heap sizes lowered: api 6→1 GB, worker/nuq-worker/extract 3→0.75 GB, prefetch 2→0.4 GB. New `global:` block in `values.yaml`.
- `k8s/charts/firecrawl/templates/playwright-deployment.yaml`: added a `resources:` block gated on `.Values.resources.enabled` so playwright honours per-service overrides (was the only workload running unbounded; got OOMKilled until this landed).
- `k8s/charts/support/templates/deployment.yaml`: added an optional `tolerations:` block via `{{- with .tolerations }}` so support-chart deployments can tolerate the control-plane taint (used by the new camofox Application).
- `manifests/values/argocd.yaml`: `configs.cm.resource.exclusions` excluding every `*.gcp.upbound.io` apiGroup (`cloudplatform`, `iam`, `storage`, `gcp.upbound.io`) from ArgoCD's cluster cache sync. The Crossplane GCP providers' conversion webhooks were 404'ing every `ComparisonError` chain, blocking every Argo Application. Excluding decouples Argo from that outage.

### Changed
- `k8s/charts/hermes/values.yaml`: `extraContainers` set to `[]` (was the inline camofox sidecar). `CAMOFOX_URL` repointed from `http://localhost:9377` to `http://camofox.camofox.svc.cluster.local:9377`. Added `FIRECRAWL_API_URL=http://firecrawl-api.firecrawl.svc.cluster.local:3002`. Hermes' web tools auto-pick `FIRECRAWL_API_URL` over `FIRECRAWL_API_KEY` when both are set.
- `k8s/applications/camofox.yaml`: trimmed to hobby sizing — `requests: 50m / 256Mi`, `limits: 500m / 512Mi`, `MAX_OLD_SPACE_SIZE=384` (idle Camoufox RSS is ~165 MB).
- `k8s/applications/firecrawl.yaml`: per-service overrides slashed — api 256Mi/1Gi, worker 256Mi/768Mi, nuq-worker 384Mi/768Mi, nuq-prefetch-worker 128Mi/384Mi, rabbitmq 256Mi/512Mi, nuq-postgres 128Mi/512Mi, playwright 256Mi/1Gi. Net memory request drop ~1.5 GB on dell01 plus one fewer pod (extractWorker disabled).

### Removed
- The in-pod camofox sidecar from the hermes StatefulSet (commented out in `values.yaml`, then replaced entirely by `extraContainers: []`). Hermes-0 is now `1/1` not `2/2`; camofox lives in its own namespace.
- `extractWorker` deployment from firecrawl (AI-extract feature unused). One fewer pod on dell01.

## 2026-06-03 Hermes Chart Stabilization, Refactoring, and Dynamic File Sync
- Session ID: 019e7c2b-2148-75bd-8a97-f3d8a975f5af
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-05-31T03-54-21-896Z_019e7c2b-2148-75bd-8a97-f3d8a975f5af.jsonl
- Session Name: hermes-chart-stabilization
- Context Name: undefined

### Added
- `k8s/images/hermes/op-files-sync`: Python script that dynamically downloads ALL file attachments from a 1Password item via the Connect REST API (lists item files, downloads each by name). No filenames configured anywhere — fully dynamic.
- `k8s/charts/hermes/templates/extra-objects.yaml`: fixed YAML document separator bug that concatenated `---apiVersion:` on one line when 2+ extraObjects existed.
- `k8s/charts/hermes/values.yaml` — git-login init container: authenticates gh CLI + configures git credential helper at startup using the token from hermes-env secret (must `unset GH_TOKEN` first or gh refuses to persist).
- `k8s/charts/hermes/values.yaml` — op-files init container: calls `op-files-sync` to download 1Password file attachments into `/opt/data/files` using the Connect token from the cluster's `op-credentials` secret (via agent's cluster-read RBAC).
- `k8s/charts/hermes/values.yaml` — dedicated 10Gi `hermes-obsidian` PVC (extraObjects) mounted at `/obsidian`, separate from hermes state volume. `OBSIDIAN_PATH=/obsidian` env override.
- `home-systems-values/hermes/values.yaml` (private repo): Discord/WhatsApp identifiers (channel IDs, phone numbers) as env vars, loaded via ArgoCD `$values` multi-source.
- `.gitignore`: added `__pycache__/` and `*.pyc` entries.

### Changed
- `k8s/applications/hermes.yaml`: refactored from ~250 lines of inline valuesObject to just 3 keys: `virtualService.hosts` (hermes.syscd.tech), `nodeSelector` (dell01), `tolerations` (control-plane). Converted to multi-source with `$values` ref to private `home-systems-values` repo.
- `k8s/charts/hermes/values.yaml`: absorbed all defaults from the Application (security contexts, RBAC, networkPolicy, tenantIsolation, PDB, camofox sidecar, externalSecret binding to hermes-env with dataFrom.extract, litellm OPENAI_BASE_URL, CAMOFOX_URL, resources, persistence 20Gi longhorn-ha, bootstrap.overwrite=false, virtualService default disabled). Made chart self-valid against its own schema.
- `k8s/images/hermes/Dockerfile`: added `COPY op-files-sync /usr/local/bin/op-files-sync` for the dynamic file sync script.
- `.bin/hermes-helper`: updated container name from `hermes` to `hermes-agent` and secret name from `hermes` to `hermes-env` (local only, gitignored).

### Removed
- `k8s/charts/hermes/values.yaml`: removed `migrate-perms` init container (privileged root chown) — migration was complete, data already owned 1000:1000.
- `k8s/charts/hermes/templates/files-external-secret.yaml`: replaced by the dynamic op-files init approach (ESO extract can't retrieve 1Password file attachments).
- Removed hermes-files secret volume mount (no longer needed; files written directly to PVC by init).

## 2026-05-31 Crossplane GCP + external-dns Cloudflare Migration; istio-gateway Sync Fix
- Session ID: 019e79b5-eed4-75d3-8f93-824723f8dddd
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-05-30T16-27-06-836Z_019e79b5-eed4-75d3-8f93-824723f8dddd.jsonl
- Session Name: 2026-05-31-2240-hermes-config-secrets
- Context Name: 2026-05-31-2240-hermes-config-secrets

### Changed
- `k8s/applications/istio-gateway.yaml`: added `helm.skipSchemaValidation: true`. The istio `gateway` chart 1.26.2 ships a `values.schema.json` that the newer Helm used by ArgoCD rejects (even defaults fail on `_internal_defaults_do_not_set`), leaving the app unsyncable; skipping schema validation lets it render and sync again.

### Notes
- Briefly pinned the gateway to `dell01` (nodeSelector + control-plane toleration) on the theory that only dell01 is on the LAN for MetalLB L2; **reverted** after MetalLB `servicel2status` showed dell01 produces no L2 announcement for the `.201` VIP, which killed reachability. The gateway is reachable on tp1/tp4 (HTTP/80 → 301); net change to the file is only `skipSchemaValidation`.
- Other work this session lives outside this repo: GCP resources migrated to native Crossplane (charts `crossplane-providers`/`crossplane-gcp`, committed in a prior session block), Cloudflare DNS moved to external-dns `DNSEndpoint`s (chart `dns-records`, `external-dns.yaml` `allowInsecureImages` fix), private values repo `home-systems-values`, and 1Password items `crossplane-gcp` + `argocd-home-systems-values`.
- UNRESOLVED: istio-gateway HTTPS/443 resets for all hosts (HTTP/80 works). Pre-existing; ruled out DNS, cert, node, MetalLB, ambient, ztunnel. See context `2026-05-31-2240-hermes-config-secrets`.

## 2026-05-31 Remote Kube Access Over Tailscale + nostos Dual-Context Kubeconfig
- Session ID: 019e7f8d-841f-725c-a533-a62f11abe6de
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-05-31T19-40-41-375Z_019e7f8d-841f-725c-a533-a62f11abe6de.jsonl
- Session Name: tailscale
- Context Name: tailscale

### Added
- `cluster.tailscale_operator` config field (`.submodules/nostos/internal/config/config.go`): optional Tailscale operator hostname; empty disables. Set to `tailscale-operator` in `nostos/config.yaml`.
- `cluster.ConfigureTailscaleContext` (`.submodules/nostos/internal/cluster/bootstrap.go`): after the talosctl kubeconfig fetch, runs `tailscale configure kubeconfig <host>` (with `KUBECONFIG` pointed at `~/.talos/kubeconfig`) to add the remote API-server-proxy context alongside the LAN context, then restores `admin@talos-default` as the default current-context. Includes `kubeconfigCurrentContext`/`setKubeconfigCurrentContext` helpers that rewrite only the `current-context` key via yaml.Node. Best-effort: warns if the `tailscale` CLI is absent.
- Two tests in `internal/cluster/bootstrap_test.go` (disabled no-op; adds remote context + restores LAN default using a faked `tailscale` binary).

### Changed
- `k8s/applications/tailscale.yaml`: enabled the operator's in-process API server proxy in auth mode (`apiServerProxyConfig.mode: "true"`), exposing kube-apiserver over the tailnet at `tailscale-operator:443` with Kubernetes RBAC + tailnet-identity impersonation. Committed `7197f842`, pushed, ArgoCD-synced.
- `.submodules/nostos/internal/cli/commands.go`: wired `ConfigureTailscaleContext` into the `kubeconfig` and `bootstrap` commands (reports `tailscale context added: …`; JSON output gains `tailscale_context`/`tailscale_warning`).

### Notes
- Cert provisioning for the proxy initially failed with ACME DNS-01 `SetDNS ... 500 failed to create DNS record`; resolved by deleting the stale offline tailnet device `tailscale-operator-2`. Verified `kubectl get nodes` and `auth can-i '*' '*'` over the proxy.
- Tailnet grant added in the admin console (not in-repo): `autogroup:admin` -> `tag:k8s-operator` impersonating `system:masters`.

## 2026-05-31 Talos v1.13.3 Upgrade And Longhorn Storage Capacity
- Session ID: 019e7bf4-f940-7ded-a97e-f21d6b006f25
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-05-31T02-55-12-704Z_019e7bf4-f940-7ded-a97e-f21d6b006f25.jsonl
- Session Name: 2026-05-31-1306-dashboard-mock-and-ci
- Context Name: 2026-05-31-1306-dashboard-mock-and-ci

### Added
- `nostos upgrade` command (`.submodules/nostos/internal/cli` + `internal/upgrade/`): auto-detects each node's Talos version, fetches the stable release catalog from GitHub, computes the adjacent-minor step path, orders workers-first/control-plane-last, and runs health-gated rolling upgrades. Includes an interactive Bubble Tea TUI (`internal/upgrade/tui`).
- `internal/upgrade/toolcache.go` (+ test): downloads and caches a `talosctl` binary matching each node's current version per hop, fixing the `too_many_pings` GoAway when a newer client talks to an older server.
- nostos render now templates `install.image` from config (`{{ .InstallImage }}` = `factory.talos.dev/metal-installer/<schematic>:<version>`), so version/schematic live only in `config.yaml`.
- `nostos/templates/dell01.yaml`: `machine.disks` partitioning `/dev/sda` (wiped 256GB SATA) mounted at `/var/mnt/storage`, plus kubelet `extraMount` for it.
- `docs/mock-dashboard.html` — interactive HTML simulator of the proposed nostos dashboard: tabbed Charm-v2 shell (Overview/Nodes/Upgrade/Network/Playbooks), live upgrade state machine (nodes flip version with progress bars, cluster heals from degraded→healthy), command palette (`:` / ⌘K), per-disk usage breakdown, full node-detail view, and auto-detect provisioning (Dell PXE / new RK1). Notifies on completion only; demo/simulate controls live outside the TUI frame.

### Changed
- `nostos/config.yaml`: `talos_version` v1.10.3 → v1.13.3; schematics bumped to add `iscsi-tools` + `util-linux-tools` (amd64 `8f04ea6b…`, arm64 `6f9371bc…`).
- `k8s/applications/longhorn.yaml`: removed a duplicate `defaultSettings:` block that silently dropped `defaultDataPath` (Longhorn had been stuck on the 28GB OS partition); added control-plane tolerations + `taintToleration` so dell01 joins Longhorn.
- `internal/cluster/bootstrap_test.go`: `t.Setenv("HOME", tmp)` so the test no longer clobbers the operator's real `~/.talos/config`/`kubeconfig`.
- Executed: cluster upgraded v1.10.3 → v1.11.6 → v1.12.8 → v1.13.3 (all 3 nodes); Longhorn migrated to big disks (tp1 NVMe 255GB, dell01 SATA 255GB) via live disk eviction, zero data loss.
- Migrated ALL apps off `local-path` onto Longhorn: `k8s/applications/{home-assistant,zigbee2mqtt,foundry}.yaml` → `longhorn`/`longhorn-ha` (and unpinned home-assistant/zigbee2mqtt from the `syscd.dev/storage: tp1` nodeSelector); `k8s/charts/bind9` default storageClass → `longhorn-ha`; `k8s/charts/support` PVC template storageClassName now configurable (default `longhorn-ha`) with optional `volumeName`.
- Runtime cutover: rsynced home-assistant + zigbee2mqtt data from old local-path PVs into new Longhorn PVCs (zero data loss); bind9 recreated fresh (data discarded per user, external-dns repopulates); foundry recreated fresh on Longhorn (was defunct/Pending — GPU node gone). Result: 0 local-path PVCs/PVs cluster-wide, all 10 PVs on longhorn/longhorn-ha.

### Removed
- `k8s/charts/echotube/templates/pvc.yaml` — echotube no longer uses a PVC; `deployment.yaml` now mounts `emptyDir` for `/app/cache`.
- `k8s/charts/support-cluster/templates/volumes-tp1.yaml` — deleted the static local PVs (home-assistant/zigbee2mqtt/node-red/teleport/appdaemon on tp1's `/var/mnt/storage`).
- local-path-provisioner removed from the GitOps flow (no pods, no `local-path` StorageClass remain).

## 2026-05-02 First 3080 Debug Session — Dell As New Control Plane Via PXE
- Session ID: 019da35b-d9dc-746e-b542-9e9f1d4b2c1d
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-04-19T01-29-59-004Z_019da35b-d9dc-746e-b542-9e9f1d4b2c1d.jsonl
- Session Name: first 3080 debug session
- Context Name: first 3080 debug session

### Added
- `pxe/` directory with README, `schematic-amd64.yaml` (Image Factory schematic for Tailscale+amd64), `nodes.yaml` (MAC/IP/role registry), and `templates/dell01.yaml` (control-plane machineconfig with `op://` refs reusing the kubernetes vault secrets).
- `scripts/pxe/detect-mac-ip.sh` — picks the Mac's ethernet interface on 192.168.68.0/24 (skips Wi-Fi).
- `scripts/pxe/1-build-assets.sh` — downloads Talos v1.10.3 kernel+initramfs, clones+builds iPXE `snponly.efi` (267KB, under the Dell UEFI TFTP ceiling) with an embedded `dhcp; chain <Mac>:9080/boot.ipxe`, renders top-level `boot.ipxe` referencing the current Mac IP.
- `scripts/pxe/2-render-config.sh` — `op inject`s a node template into `pxe/assets/configs/<mac-hex-hyphens>.yaml` so iPXE `${mac:hexhyp}` fetches the right config.
- `scripts/pxe/3-serve.sh` — starts Python HTTP:9080 + dnsmasq DHCP/TFTP on the detected ethernet; kills stale HTTP on port 9080; fast-fails with a clear error if passwordless sudo isn't set up.
- `taskfiles/pxe.yml` — `task pxe:setup`, `pxe:config NODE=`, `pxe:up`, `pxe:down`, `pxe:status`, `pxe:clean-assets`; wired into root `Taskfile.yml`.
- `docs/pxe-boot.md` — full troubleshooting notes: Dell BIOS settings, Tailscale network-extension interference on macOS, iPXE binary size limits, Secure Boot, Deco router DHCP race.

### Changed
- `talos/controlplane-192.168.68.100.yaml`: `machine.type` flipped from deprecated `init` to `controlplane`.
- `.gitignore`: ignore `pxe/assets/` (downloaded binaries + rendered secret-bearing configs) and `pxe/ipxe-src/` (iPXE build tree).
- `pxe/templates/dell01.yaml`: sanitized comments — removed literal `op://...` substrings that were triggering `op inject` matches.

## 2026-05-02 PXE Boot Script Fixes For macOS dnsmasq
- Session ID: 019de8d9-f5c8-765c-b738-f2c596a458a3
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-05-02T13-21-31-593Z_019de8d9-f5c8-765c-b738-f2c596a458a3.jsonl
- Session Name: 2026-05-02-1312-pxe-boot-talos-setup
- Context Name: 2026-05-02-1312-pxe-boot-talos-setup

### Changed
- `scripts/pxe/3-serve.sh`: sudo precheck now runs `sudo -n dnsmasq --version` instead of `sudo -n true`, so a NOPASSWD sudoers entry scoped to dnsmasq actually satisfies it.
- `scripts/pxe/3-serve.sh`: TFTP root staged at `/tmp/pxe-tftp` with `ipxe.efi` copied and chmodded 755/644 on every start. Needed because `/Users/yuri` is 0750 and dnsmasq drops privileges to `nobody`, which couldn't traverse into the repo to read `pxe/assets/ipxe.efi`.
- `scripts/pxe/3-serve.sh`: removed per-MAC `--dhcp-host` pinning and the `pxe/nodes.yaml` scrape loop. Added `--dhcp-match=set:pxe,60,PXEClient` and `--dhcp-ignore=tag:!pxe` so dnsmasq only answers PXE clients, not arbitrary LAN devices (avoids fighting the Deco router's DHCP).
- `pxe/nodes.yaml`: dell01 `ip` corrected from `192.168.68.100` (outside the `.200-.210` dhcp-range) to `192.168.68.200`. No longer consumed by `3-serve.sh` but left accurate.

### Added
- `.agents/tmp/pxe-diff.html`: side-by-side HTML diff of the working manual `sudo dnsmasq ...` invocation vs the script-generated invocation used during triage.
