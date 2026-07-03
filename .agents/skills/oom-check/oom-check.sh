#!/usr/bin/env bash
# oom-check — find containers killed/restarting due to too-low memory limits,
# and suggest a corrected limit from VPA evidence. GitOps cluster: report only,
# never applies anything.
set -euo pipefail

CTX="${KUBE_CONTEXT:-admin@talos-default}"
APISERVERS=(192.168.68.114 192.168.68.107 192.168.68.100)

# Find a reachable apiserver (VIP round-robins across possibly-down CPs).
kc() {
  local ep tries
  for ep in "${APISERVERS[@]}"; do
    for tries in 1 2 3; do
      if out=$(kubectl --context "$CTX" --server="https://${ep}:6443" \
                 --insecure-skip-tls-verify "$@" 2>/dev/null) && [ -n "$out" ]; then
        printf '%s' "$out"; return 0
      fi
      sleep 2
    done
  done
  echo "ERROR: no reachable apiserver on ${APISERVERS[*]}" >&2
  return 1
}

echo "Fetching pods + VPA recommendations (retrying flaky apiserver)..." >&2
TMPD=$(mktemp -d)
trap 'rm -rf "$TMPD"' EXIT
kc get pods -A -o json > "$TMPD/pods.json"
kc get vpa -A -o json 2>/dev/null > "$TMPD/vpa.json" || echo '{"items":[]}' > "$TMPD/vpa.json"

PODS_FILE="$TMPD/pods.json" VPA_FILE="$TMPD/vpa.json" python3 - <<'PY'
import json, os, math

pods = json.load(open(os.environ["PODS_FILE"]))
vpa  = json.load(open(os.environ["VPA_FILE"]))

def to_mi(v):
    if not v: return None
    v = str(v)
    units = {"Ki":1/1024, "Mi":1, "Gi":1024, "Ti":1024*1024,
             "k":1/1024, "M":0.953674, "G":953.674, "T":953674}
    for s in ("Ki","Mi","Gi","Ti","k","M","G","T"):
        if v.endswith(s):
            try: return float(v[:-len(s)])*units[s]
            except: return None
    try: return float(v)/1024/1024   # raw bytes
    except: return None

# VPA steady-state target memory per (ns, workload-ish, container)
vtarget = {}
for it in vpa.get("items", []):
    ns = it["metadata"]["namespace"]
    for c in (it.get("status",{}).get("recommendation",{}) or {}).get("containerRecommendations",[]) or []:
        vtarget[(ns, c["containerName"])] = to_mi(c.get("target",{}).get("memory"))

rows = []
for p in pods.get("items", []):
    ns = p["metadata"]["namespace"]
    limits = {c["name"]: (c.get("resources",{}).get("limits",{}) or {}).get("memory")
              for c in p["spec"].get("containers",[])}
    for cs in p.get("status",{}).get("containerStatuses",[]) or []:
        name = cs["name"]
        restarts = cs.get("restartCount",0)
        last = (cs.get("lastState",{}).get("terminated",{}) or {}).get("reason")
        oom = last == "OOMKilled"
        if not oom and restarts < 3:
            continue
        lim = limits.get(name)
        tgt = vtarget.get((ns, name))
        sug = None
        if tgt:
            sug = int(math.ceil(tgt*1.5/128)*128)  # target*1.5, round to 128Mi
        rows.append((ns, f"{p['metadata']['name']}/{name}", restarts,
                     last or "-", lim or "none",
                     f"{int(tgt)}Mi" if tgt else "-",
                     f"{sug}Mi" if sug else "-", oom))

rows.sort(key=lambda r: (not r[7], -r[2]))
if not rows:
    print("No OOMKilled or high-restart containers found. Clean.")
else:
    print(f'{"NAMESPACE":16} {"POD/CONTAINER":48} {"RST":>4} {"LASTREASON":11} {"LIMIT":>7} {"VPA_TGT":>8} {"SUGGEST":>8}')
    for r in rows:
        flag = "  <-- OOMKilled" if r[7] else ""
        print(f'{r[0]:16} {r[1][:48]:48} {r[2]:>4} {r[3]:11} {r[4]:>7} {r[5]:>8} {r[6]:>8}{flag}')
    print()
    print("GitOps: set SUGGEST in the workload's chart/Application; do NOT kubectl apply.")
PY
