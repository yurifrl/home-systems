#!/usr/bin/env bash
# rightsize.sh — right-size the platform from VPA (Goldilocks) recommendations.
#
# Read-only report. Joins each Goldilocks VPA's recommended `target` (cpu/mem)
# against the workload's CURRENT requests so you can set requests/limits that
# track real consumption.
#
# Usage:
#   hack/rightsize.sh                 # table, all namespaces
#   hack/rightsize.sh <namespace>     # table, one namespace
#   hack/rightsize.sh -o values <ns>  # emit a resources: snippet per container
#
# Notes:
#   - "rec" is VPA's recommended request (target). The suggested memory LIMIT is
#     the VPA upperBound (spike headroom); CPU is left limitless on purpose.
#   - Trust requires a healthy metrics pipeline (metrics-server + vmsingle) AND
#     a few days of clean data. Treat <1d of history as provisional.
set -euo pipefail

MODE="table"
if [[ "${1:-}" == "-o" ]]; then MODE="${2:-table}"; shift 2; fi
NS_FILTER="${1:-}"

vpa_f=$(mktemp); wl_f=$(mktemp)
trap 'rm -f "$vpa_f" "$wl_f"' EXIT
kubectl get vpa -A -o json > "$vpa_f"
kubectl get deploy,statefulset,daemonset -A -o json > "$wl_f"

out=$(jq -nr \
  --arg nsf "$NS_FILTER" \
  --arg mode "$MODE" \
  --slurpfile vpa "$vpa_f" \
  --slurpfile wl "$wl_f" '
  def hmem(v):
    if v == null then "-"
    elif (v|tostring|test("^[0-9]+$")) then ((v|tonumber)/1048576|floor|tostring) + "Mi"
    else (v|tostring) end;
  def hcpu(v): if v == null then "-" else (v|tostring) end;

  ( [ $wl[0].items[]
      | .metadata.namespace as $ns | .kind as $k | .metadata.name as $n
      | (.spec.template.spec.containers[]
         | { key: ($ns+"|"+$k+"|"+$n+"|"+.name),
             value: (.resources.requests // {}) } )
    ] | from_entries ) as $req

  | $vpa[0].items[]
  | select($nsf=="" or .metadata.namespace==$nsf)
  | .metadata.namespace as $ns
  | .spec.targetRef.kind as $k
  | .spec.targetRef.name as $n
  | (.status.recommendation.containerRecommendations // [])[]
  | .containerName as $c
  | ($req[$ns+"|"+$k+"|"+$n+"|"+$c] // {}) as $cur
  | { ns:$ns, wl:($k+"/"+$n), c:$c,
      cur_cpu:hcpu($cur.cpu), cur_mem:hmem($cur.memory),
      rec_cpu:hcpu(.target.cpu), rec_mem:hmem(.target.memory),
      lim_mem:hmem(.upperBound.memory),
      flag:(if ($cur.cpu==null and $cur.memory==null) then "NO-REQ" else "ok" end) }
  | if $mode=="values" then
      "        - name: \(.c)\n          requests: { cpu: \(.rec_cpu), memory: \(.rec_mem) }\n          limits:   { memory: \(.lim_mem) }"
    else
      [.ns, .wl, .c, (.cur_cpu+"->"+.rec_cpu), (.cur_mem+"->"+.rec_mem), .flag] | @tsv
    end
')

if [[ "$MODE" == "table" ]]; then
  { printf 'NAMESPACE\tWORKLOAD\tCONTAINER\tCPU(cur->rec)\tMEM(cur->rec)\tFLAG\n'; printf '%s\n' "$out"; } \
    | column -t -s $'\t'
else
  printf '%s\n' "$out"
fi
