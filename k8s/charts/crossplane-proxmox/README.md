# crossplane-proxmox

Replicates the hand-written Proxmox VM configs in [`proxmox/`](../../../proxmox)
as Crossplane managed resources, so the VMs on the Proxmox host
(`pc01`, `https://192.168.68.112:8006/`) are described declaratively and
reconciled by Crossplane instead of being hand-edited.

| Source config        | VM id | Managed resource (`EnvironmentVM`) |
|----------------------|-------|------------------------------------|
| `proxmox/100.conf`   | 100   | `talos-pc01` (`templates/talos-pc01.yaml`) |
| `proxmox/101.conf`   | 101   | `windows`    (`templates/windows.yaml`)    |

## How it fits the repo

- Provider package `provider-proxmox-bpg` is declared in
  [`crossplane-providers`](../crossplane-providers/values.yaml) (sync-wave `-1`).
- This chart (sync-wave `0`) renders the `ProviderConfig` + the two
  `EnvironmentVM` managed resources.
- The ArgoCD `Application` is [`k8s/applications/crossplane-proxmox.yaml`](../../applications/crossplane-proxmox.yaml),
  auto-discovered by the `applications` ApplicationSet.
- Provider: `xpkg.upbound.io/valkiriaaquaticamendi/provider-proxmox-bpg`
  (Upjet wrapper around the `bpg/terraform-provider-proxmox` Terraform provider).
  Managed-resource group: `virtualenvironmentvm.proxmoxbpg.crossplane.io`.

## Credentials

The provider reads one Secret key holding a JSON blob. By default an
`ExternalSecret` materialises it from 1Password (ClusterSecretStore
`onepassword`, item `crossplane-proxmox`). The 1Password property must contain:

```json
{
  "endpoint": "https://192.168.68.112:8006/",
  "username": "root@pam",
  "password": "<password>",
  "insecure": "true",
  "ssh_username": "root"
}
```

Prefer a Proxmox **API token** (`"api_token": "root@pam!crossplane=<uuid>"`)
over the root password for real use.

## ⚠️ Known limitations / prerequisites (verified 2026-06-10)

These were validated live against `pc01` and the provider `v1.15.0`:

1. **VM creation requires the referenced ISOs to already exist on the host.**
   The provider's `qmcreate` validates ISO volumes at create time
   (`unable to create VM 100 - volume 'local:iso/talos-1.10.0-metal-amd64.iso'
   does not exist`). Upload the ISOs to the `local` datastore first:
   - `talos-1.10.0-metal-amd64.iso`
   - `Win11_24H2_English_x64.iso`
   - `virtio-win.iso`

2. **VM 101 (`windows`) cannot be replicated faithfully with this provider.**
   `proxmox/101.conf` defines **two CD-ROM drives** (`ide0` = virtio-win,
   `ide2` = Win11), but `provider-proxmox-bpg` permits only a single `cdrom`
   block (`Too many cdrom blocks: No more than 1 "cdrom" blocks are allowed`).
   The `windows` template is therefore left disabled / incomplete pending a
   decision (single boot ISO only, mount virtio-win post-install, or a
   different provisioning path).

3. **Both VMs passthrough the same GPU** (`0000:01:00.0`) and cannot run
   simultaneously, so `started` defaults to `false`.
