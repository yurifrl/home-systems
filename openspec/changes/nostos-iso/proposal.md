# nostos-iso

## Why

Provisioning the Windows guest VM on the Proxmox host depends on a custom combined install ISO (Win11 + virtio drivers + autounattend + no-keypress boot) that today is produced and published by loose, hardcoded shell scripts (`hack/win11-iso/*`). Those scripts bake in machine-specific values (bucket name, GCP project, object name), so they can't serve another machine and drift from the rest of the config-driven workflow. Folding this into nostos makes guest install media a first-class, config-driven capability with no hardcoded machine identity.

## What Changes

- Add a `nostos iso` command group that builds, publishes, and serves guest install ISOs:
  - `nostos iso build <name>` — build the combined ISO for the named guest (shells to a privileged container running an embedded build script).
  - `nostos iso publish <name>` — upload the built ISO to the configured object store (GCS), credentials resolved via the existing `op://` secrets backend.
  - `nostos iso url <name>` — mint a short-lived V4 signed URL for the object and print a paste-ready snippet for the consuming config (e.g. the crossplane-proxmox private values).
  - `nostos iso prepare <name>` — build → publish → url end-to-end.
- Add a config-driven `images` (guest install media) section to nostos `config.yaml`: per-named-entry build source (UUP id, edition, driver/answer-file inputs), object store target (bucket, object, project), and `op://` credentials reference. **No machine names, bucket names, or project ids are hardcoded in Go** — everything is resolved from config keyed by `<name>`.
- Remove the loose `hack/win11-iso/` scripts and their hardcoded references once the command reaches parity.

## Capabilities

### New Capabilities
- `guest-iso`: build, publish, and sign URLs for custom guest-VM install ISOs, driven entirely by named config entries (loose coupling, no hardcoded machine identity).

### Modified Capabilities
<!-- none: nostos currently has no spec'd capability for guest install media -->

## Impact

- **nostos** (`.submodules/nostos`): new `internal/cli` command group (`iso`), a new `images` config schema + validation, a new builder/publisher package, an embedded build script asset, and a new dependency on the GCS SDK (`cloud.google.com/go/storage`). Adds a Docker runtime assumption for the `build` verb.
- **home-systems**: `hack/win11-iso/` scripts removed; `crossplane-proxmox` docs/comments point at `nostos iso` instead. The crossplane chart continues to consume the published ISO via signed URL (unchanged contract).
- **Secrets/storage**: reuses the existing `op://` 1Password backend for GCS credentials; the private `iso-images` bucket (Crossplane-managed) is unchanged.
