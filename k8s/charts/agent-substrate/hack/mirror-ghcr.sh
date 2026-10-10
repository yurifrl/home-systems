#!/usr/bin/env bash
# Copy the mirrored component images from company ECR into public ghcr.io/yurifrl
# for the home talos cluster. Digests are preserved (plain crane copy), so chart
# values only swap the registry prefix.
#
# Usage: GHCR_TOKEN=<PAT with write:packages> ./mirror-ghcr.sh
# Reads ECR with the `ai` profile (read-only). Also lands the ateom-gvisor
# worker image, which was ko-built straight into the local kind registry and
# never pushed to ECR; falls back to a ko build when the kind registry is gone.
set -euo pipefail

ECR=710045723375.dkr.ecr.us-east-1.amazonaws.com
GHCR=ghcr.io/yurifrl
KINDREG=localhost:5001

: "${GHCR_TOKEN:?GHCR_TOKEN (classic PAT with write:packages) is required}"

export DOCKER_CONFIG=${DOCKER_CONFIG:-/tmp/.dockercfg-mirror}
aws --profile ai ecr get-login-password --region us-east-1 |
  crane auth login "$ECR" -u AWS --password-stdin
printf '%s' "$GHCR_TOKEN" | crane auth login ghcr.io -u yurifrl --password-stdin

copy() { crane copy "$1" "$2"; }

# --- substrate control plane + infra (built by hack/mirror-images.sh into ECR)
copy "$ECR/ate/ateapi:a0b680d"                  "$GHCR/ate/ateapi:a0b680d"
copy "$ECR/ate/atecontroller:a0b680d"           "$GHCR/ate/atecontroller:a0b680d"
copy "$ECR/ate/atelet:a0b680d"                  "$GHCR/ate/atelet:a0b680d"
copy "$ECR/ate/podcertcontroller:a0b680d"       "$GHCR/ate/podcertcontroller:a0b680d"
copy "$ECR/ate/agentgateway:v0.0.0-alpha.9f9744cf" "$GHCR/ate/agentgateway:v0.0.0-alpha.9f9744cf"
copy "$ECR/ate/postgres:18-alpine"              "$GHCR/ate/postgres:18-alpine"
copy "$ECR/ate/rustfs:1.0.0-beta.3"             "$GHCR/ate/rustfs:1.0.0-beta.3"
copy "$ECR/ate/aws-cli:2.17.0"                  "$GHCR/ate/aws-cli:2.17.0"
copy "$ECR/ate/otel-collector-contrib:0.157.0"  "$GHCR/ate/otel-collector-contrib:0.157.0"
copy "$ECR/ate/jaeger:1.55"                     "$GHCR/ate/jaeger:1.55"

# --- ax (server, controller, task-runner, redis)
copy "$ECR/ax/ax-server:c5c1ac5"                "$GHCR/ax/ax-server:c5c1ac5"
copy "$ECR/ax/ax-controller:c5c1ac5"            "$GHCR/ax/ax-controller:c5c1ac5"
copy "$ECR/ax/ax-task-runner:c5c1ac5"           "$GHCR/ax/ax-task-runner:c5c1ac5"
copy "$ECR/ax/redis:7-alpine"                   "$GHCR/ax/redis:7-alpine"

# --- ateom gVisor worker image: only ever pushed to the local kind registry.
# ko default namer: <KO_DOCKER_REPO>/ateom-gvisor-<md5(import path)>:<tag>.
# After crane copy it lands under the canonical ghcr name the WorkerPool CR
# references (workerImage: ghcr.io/yurifrl/ate/ateom-gvisor:a0b680d).
if crane digest "$KINDREG/ate/ateom-gvisor-715889664656de67e44382a8d6ab981d:a0b680d" >/dev/null 2>&1; then
  copy "$KINDREG/ate/ateom-gvisor-715889664656de67e44382a8d6ab981d:a0b680d" \
       "$GHCR/ate/ateom-gvisor:a0b680d"
else
  # Kind registry unavailable: rebuild from source.
  SUBSTRATE_SRC=${SUBSTRATE_SRC:-$(cd "$(dirname "$0")/../../../../substrate-src" && pwd)}
  (cd "$SUBSTRATE_SRC" && DOCKER_CONFIG="$DOCKER_CONFIG" \
    KO_DOCKER_REPO="$GHCR/ate" ko build --sbom=none -t a0b680d ./cmd/ateom-gvisor)
fi

echo "mirror complete:"
for r in ateapi atecontroller atelet podcertcontroller agentgateway postgres rustfs aws-cli otel-collector-contrib jaeger ateom-gvisor; do
  printf '  %s/ate/%s\n' "$GHCR" "$r"
done
for r in ax-server ax-controller ax-task-runner redis; do
  printf '  %s/ax/%s\n' "$GHCR" "$r"
done
