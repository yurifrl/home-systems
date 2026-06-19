# Win11 combined install ISO — build, store, and serve to Proxmox

The Windows VM (`crossplane-proxmox` chart, VM 101) installs from a **single
combined ISO**, because `provider-proxmox-bpg` allows only one CD-ROM. That ISO
bundles everything Setup needs:

- Windows 11 24H2 retail amd64 (built from UUP dump)
- VirtIO `viostor` + `NetKVM` drivers under `\virtio` (so Setup sees the `virtio0` disk)
- `autounattend.xml` at the root (hands-free install — see `../../k8s/charts/crossplane-proxmox/files/autounattend.xml`)
- no-keypress UEFI boot (`efisys_noprompt.bin`) so there's no "press any key to boot from CD"

It lives in the **private** GCS bucket `iso-images`
(`syscd-iso-images-…`, public-access-prevention ENFORCED), and Proxmox pulls it
via a **V4 signed URL** that Crossplane's `EnvironmentDownloadFile` uses for the
one-time download.

```
build (Docker) ──▶ gsutil upload ──▶ gs://iso-images/Win11_24H2_combined.iso (PRIVATE)
                                              │  signed URL (≤7d)
                                              ▼
                       EnvironmentDownloadFile ──▶ Proxmox local:iso ──▶ VM 101 boots & autoinstalls
```

## What's automated vs manual

| Step | Automated? |
|------|------------|
| Proxmox **downloads** the ISO from the bucket | ✅ Crossplane (`EnvironmentDownloadFile`) — re-pulls on host wipe, **while the signed URL is valid** |
| Signed URL validity | ⚠️ max **7 days** — regenerate with `sign-url.sh` for any re-download after that |
| **Building + uploading** the ISO | ❌ manual — run `build.sh` + `gsutil cp` (only needed when the ISO content changes or the bucket object is lost) |

Once the ISO is resident on Proxmox the download MR stays `Ready` even after the
URL expires (the provider observes the resident file, not the URL — verified).

## 1. Build the ISO (only when content changes)

Runs in a privileged `debian:13` container (loop-mounts ISOs); reproducible on
any machine with Docker/OrbStack. `~20 min`, ~4 GB download.

```bash
REPO=$(git rev-parse --show-toplevel)
mkdir -p /tmp/iso-build/out
docker run --rm --privileged \
  -e WIN_ADMIN_PASSWORD="$(op item get windows-pc01-admin --vault kubernetes --fields password --reveal)" \
  -v "$REPO/k8s/charts/crossplane-proxmox/files:/ctx:ro" \
  -v "$REPO/hack/win11-iso/build.sh:/build.sh:ro" \
  -v /tmp/iso-build/out:/out \
  debian:13 bash /build.sh
# -> /tmp/iso-build/out/Win11_24H2_combined.iso
# (autounattend.xml's __WIN_ADMIN_PASSWORD__ is replaced at build time from the
#  1Password item windows-pc01-admin; the repo never holds the real password.)
```

## 2. Upload to the private bucket (only after a rebuild)

```bash
# auth with the GCS SA key from 1Password (item: crossplane-gcp), via a
# temp keyfile that's removed on exit (never a fixed/world-readable path).
KEY=$(mktemp); trap 'rm -f "$KEY"' EXIT
op item get crossplane-gcp --vault kubernetes --fields creds --reveal \
  | python3 -c "import sys;s=sys.stdin.read().strip();s=s[1:-1] if s.startswith('\"') else s;open('$KEY','w').write(s.replace('\"\"','\"'))"
gcloud auth activate-service-account --key-file="$KEY"
gsutil cp /tmp/iso-build/out/Win11_24H2_combined.iso \
  gs://syscd-iso-images-4c4398cd-1a7/Win11_24H2_combined.iso
gcloud auth revoke --all 2>/dev/null || true
```

## 3. Generate the signed URL and wire it in (the recurring step)

```bash
bash hack/win11-iso/sign-url.sh 7        # prints a ready-to-paste snippet
```

Paste the printed `isos.win11.url` into the **private** values repo
(`home-systems-values` → `proxmox/values.yaml`), commit, and push. ArgoCD +
Crossplane then (re)download the ISO to Proxmox and create/boot VM 101, which
installs Windows hands-free.

> The bucket name / project are pinned in `sign-url.sh`; the bucket itself is
> declared in `home-systems-values/gcp/values.yaml` (Crossplane-managed,
> `iso-images`).
