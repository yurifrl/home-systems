# Hermes chart merge — notes & cutover

Dedicated chart at `k8s/charts/hermes`, vendored from
`github.com/ultraworkers/hermes-agent-helm-chart` @ `e3b685d` and adapted for
home-systems. Replaces the `support`-chart-based `k8s/applications/hermes.yaml`.

## What's in it
- **Secrets:** `envFrom` the `hermes-env` ExternalSecret (1Password, `onepassword`
  store) — unchanged loading model. A **second** ExternalSecret `hermes-files`
  pulls 1Password **file attachments** by name and mounts them read-only at
  `/opt/data/files`.
- **Security/isolation:** non-root uid/gid/fsGroup 1000, `readOnlyRootFilesystem`,
  drop ALL caps, seccomp RuntimeDefault, NetworkPolicy (egress allowlist: DNS,
  kube API, litellm:4000, 80/443), tenantIsolation, PDB.
- **RBAC:** ClusterRole — `get/list/watch` on everything (read-only) **+ `delete`
  on pods**. `automountServiceAccountToken: true` (required to use it).
- **Integration:** GHCR image, Longhorn `longhorn-ha`, istio VirtualService
  `hermes.syscd.tech` via tailscale gateway, API server :8642, camofox sidecar,
  `OPENAI_BASE_URL`→litellm, `HOME=/opt/data/home`.
- **Image (Phase 6):** `k8s/images/hermes/Dockerfile` now adds a uid-1000 user,
  `USER 1000`, and `kubectl` (multi-arch). Renovate-pinned `KUBECTL_VERSION`.

Validation: `helm lint` clean, `helm template` renders 12 objects with the
proposed values (`k8s/applications/hermes.yaml.new`).

## Risks / watch-outs
- **read-only rootfs + uid 1000:** the wheel installs to `/usr/local/lib`
  (world-readable, fine to read). But if Hermes tries a **runtime** `pip`/`npm`
  install (lazy_deps, TUI npm) it will fail EACCES under uid 1000 + read-only fs.
  Core gateway + API + browser(camofox) + git/gh/kubectl don't need that. Watch
  the logs on first boot; if an adapter lazy-installs, pre-bake it in the image.
- First boot as uid 1000 must be able to write `/opt/data` — `fsGroup: 1000` +
  `fsGroupChangePolicy: OnRootMismatch` handles the PVC ownership.
- `hermes-files` mounts only **attachments listed** in the ExternalSecret `data:`
  (3 GCP SA JSONs today). Adding a new attachment = add a line there.

## Cutover (Phase 7) — do in order
1. **Build the hardened image first** (must land before the security context, or
   the old root image fails as uid 1000 / read-only):
   - bump touches `k8s/images/hermes/**` → `build-hermes` workflow rebuilds &
     pushes `ghcr.io/yurifrl/hermes-agent` (now with kubectl + uid 1000).
   - sanity: `docker run --rm --read-only --user 1000 ghcr.io/yurifrl/hermes-agent:latest hermes --version`
2. **Swap the Application:** `mv k8s/applications/hermes.yaml.new k8s/applications/hermes.yaml`, commit, push.
3. ArgoCD syncs the new chart; old `support`-chart resources are pruned. The
   `hermes` StatefulSet (support chart) becomes a Deployment (this chart) — the
   old PVC `state-hermes-0` is NOT reused; this chart uses PVC
   `hermes`/`persistence`. **Migrate `/opt/data` data** if you need the existing
   gh/config/memory state (copy from the old PVC, or set
   `persistence.existingClaim`).
4. Verify: pod Running, `kubectl exec ... -- hermes --version`, API `/v1/models`
   200 via the in-cluster service, files at `/opt/data/files`, `kubectl get pods`
   works from inside (RBAC), NetworkPolicy present.

## Rollback
Revert the Application file (git revert / restore the support-chart `hermes.yaml`).
The vendored chart and image changes are inert until the Application points at them.
