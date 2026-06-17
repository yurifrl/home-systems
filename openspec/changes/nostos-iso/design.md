# nostos-iso — Design

## Context

The Windows guest VM (`crossplane-proxmox` chart, VM 101) installs from a custom combined ISO: Win11 24H2 base (built via UUP dump) + virtio `viostor`/`NetKVM` drivers + `autounattend.xml` + `efisys_noprompt.bin` (no "press any key"). It must be a single ISO because `provider-proxmox-bpg` allows only one CD-ROM. Today this is produced by `hack/win11-iso/{build,sign-url}.sh`, which hardcode the bucket name, GCP project, and object name — violating the project's config-driven, no-hardcoded-machine-identity principle.

nostos already owns provisioning concerns (`internal/osimage/{proxmox,talos}` for node OS images, `internal/secrets` with an `op://` backend, a cobra command tree, and a config schema). It does **not** yet model guest install media or talk to object storage. The crossplane-proxmox chart consumes the published ISO via a V4 signed URL and that contract stays unchanged.

## Goals / Non-Goals

**Goals:**
- A config-driven `nostos iso` command group (`build`, `publish`, `url`, `prepare`) that operates on a named `images` entry.
- Zero hardcoded machine names, bucket names, object names, or project ids in Go — all resolved from config keyed by `<name>`.
- Reuse the existing `op://` secrets backend for object-store credentials; never read credentials from literals.
- Retire the loose `hack/win11-iso/` scripts at parity.

**Non-Goals:**
- Pure-Go ISO assembly (loop-mount/xorriso) — out of scope; the build shells to a container.
- Managing the GCS bucket itself (Crossplane already owns `iso-images`).
- Wiring the signed URL into Crossplane automatically — `url` emits a paste-ready snippet; committing it stays a human/GitOps step.
- Generalising beyond the current Win11 use case (the schema is generic, but only one entry is expected initially).

## Decisions

- **New `images` config section, keyed by name.** Each entry holds `build` inputs (UUP build id, edition, driver source, answer-file path), `store` target (`bucket`, `object`, `project`), and `credentialsRef` (`op://...`). Commands take `<name>` and resolve everything from the entry. *Alternative considered:* read the crossplane-proxmox chart values as the source of truth — rejected because it couples nostos to a Helm chart's layout; the chart consumes the artifact, nostos produces it (clean layering, the bucket/object name is the only shared contract).
- **Build shells out to a privileged container.** The build needs loop-mounts + `xorriso`; the proven recipe is the embedded `build.sh` run in `debian:13 --privileged`. nostos invokes the container runtime via `internal/execx`. *Alternative:* reimplement in Go with a userspace ISO library — rejected as large and fragile versus a working, embedded script.
- **Object storage + signing via the GCS SDK** (`cloud.google.com/go/storage`), with the SA key resolved through `internal/secrets` (`op://`). V4 signed URLs (≤7-day max) are minted in-process. *Alternative:* shell to `gsutil`/`gcloud` — rejected (extra runtime deps, the SDK is cleaner and testable).
- **Verb layering.** `prepare` composes `build → publish → url`, stopping at the first failure. Each verb is independently runnable so a re-sign (the common recurring need) doesn't force a rebuild.
- **Package placement.** Logic lives in a new `internal/guestiso` (or `internal/image/guest`) package behind a small interface; the cobra wiring lives in `internal/cli/iso.go` registered from `root.go`, mirroring existing command files.

## Risks / Trade-offs

- **Docker runtime assumption for `build`** → document it; `build` fails fast with a clear error if no runtime is present, and `publish`/`url` work independently of it.
- **New heavy dependency (GCS SDK) in nostos go.mod** → isolated to the guest-iso package; node-provisioning paths don't import it.
- **Signed URL expiry (≤7 days)** → `url` is cheap to re-run; documented that re-download after expiry needs a fresh URL. The download MR stays Ready post-download regardless (verified).
- **Scope creep beyond Talos provisioning** → contained to one optional command group and config section; core provisioning is untouched.
- **Privileged container** → required for loop-mounts; flagged in docs, only used by `build`.

## Migration Plan

1. Land `nostos iso` with config + tests in the submodule (no behavior change to existing commands).
2. Add an `images` entry to nostos config for the Win11 ISO (replacing the script's hardcoded values).
3. Verify `nostos iso prepare <name>` reaches parity with the scripts (ISO in bucket, valid signed URL).
4. Remove `hack/win11-iso/` and repoint crossplane-proxmox docs/comments at `nostos iso`.
   Rollback: the scripts are in git history; revert until parity holds.

## Open Questions

- Final package name (`internal/guestiso` vs `internal/image/guest`) and command noun (`iso` vs `image`) — cosmetic, decided at implementation.
- Whether `build` output path is config-declared or derived under nostos state dir.
- Whether to validate the container runtime/`op` availability in a preflight or fail lazily at the verb.
