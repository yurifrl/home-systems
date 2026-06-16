#!/usr/bin/env bash
# win11-iso-sign-url.sh — generate a V4 signed GET URL for the Win11 combined
# ISO in the PRIVATE iso-images GCS bucket, for Proxmox to download via the
# crossplane-proxmox `EnvironmentDownloadFile`.
#
# WHY THIS EXISTS
#   The iso-images bucket is private (public-access-prevention ENFORCED), so
#   Proxmox can't pull a plain URL. Crossplane's download resource only uses the
#   URL for the ONE-TIME download — once the ISO is resident on Proxmox the MR
#   stays Ready even after the URL expires (verified empirically). So you only
#   need a fresh URL when the ISO must be (re)downloaded:
#     - first deploy of the Windows VM, or
#     - a Proxmox host wipe/reinstall, or
#     - the previous signed URL is older than its TTL (max 7 days).
#
# WHAT IT DOES
#   - pulls the GCS service-account key from 1Password (item: crossplane-gcp)
#   - mints a V4 signed URL (default 7 days) for the ISO object
#   - prints the URL and a ready-to-paste snippet for the PRIVATE values repo
#
# USAGE
#   .bin/.. or: bash hack/win11-iso/sign-url.sh [days]
#
# AFTER RUNNING
#   Paste the URL into the PRIVATE values repo (home-systems-values):
#     proxmox/values.yaml -> isos.win11.url
#   commit + push; ArgoCD/Crossplane re-pulls the ISO to Proxmox.
#
# REQUIREMENTS: op (1Password CLI, signed in), python3 with google-cloud-storage
#   (pip install google-cloud-storage), network to GCS.
set -euo pipefail

DAYS="${1:-7}"
PROJECT="syscd-443112"
BUCKET="syscd-iso-images-4c4398cd-1a7"      # iso-images (private) — see home-systems-values/gcp/values.yaml
OBJECT="Win11_24H2_combined.iso"
KEYFILE="$(mktemp)"
trap 'rm -f "$KEYFILE"' EXIT

echo "# fetching GCS SA key from 1Password (crossplane-gcp)..." >&2
op item get crossplane-gcp --vault kubernetes --fields creds --reveal \
  | python3 -c "import sys;s=sys.stdin.read().strip();s=s[1:-1] if s.startswith('\"') else s;open('$KEYFILE','w').write(s.replace('\"\"','\"'))"

URL=$(python3 - "$BUCKET" "$OBJECT" "$DAYS" "$KEYFILE" "$PROJECT" <<'PY'
import sys, datetime
from google.cloud import storage
from google.oauth2 import service_account
bkt, obj, days, keyfile, project = sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4], sys.argv[5]
creds = service_account.Credentials.from_service_account_file(keyfile)
client = storage.Client(credentials=creds, project=project)
blob = client.bucket(bkt).blob(obj)
print(blob.generate_signed_url(version="v4", expiration=datetime.timedelta(days=days), method="GET"))
PY
)

echo "# signed URL for gs://$BUCKET/$OBJECT (valid ${DAYS}d):" >&2
echo
echo "# --- paste into home-systems-values/proxmox/values.yaml ---"
echo "isos:"
echo "  win11:"
echo "    enabled: true"
echo "    url: \"$URL\""
