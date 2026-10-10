#!/usr/bin/env bash
# Build/mirror agent-substrate + ax component images into ECR.
#
# Substrate (google/agent-substrate, tag = substrate.version) and ax
# (google/ax, tag = ax.version) are built with ko (Go binaries on
# distroless/static bases per each repo's .ko.yaml). agentgateway + redis are
# crane-copied from upstream. KO_DOCKER_REPO must point at the ECR
# account-level prefix (no trailing slash); each image lands in
# <KO_DOCKER_REPO>/<binary> with the requested tag.
#
# Usage:   KO_DOCKER_REPO=...account...dkr.ecr...amazonaws.com/ate ./mirror-images.sh
# Default: 710045723375.dkr.ecr.us-east-1.amazonaws.com/ate (EKS-NSX-AI)
set -euo pipefail

ATE=${KO_DOCKER_REPO:-710045723375.dkr.ecr.us-east-1.amazonaws.com/ate}
AX=${AX_REPO:-${ATE%/ate}/ax}
GW_REPO=ghcr.io/agentgateway/agentgateway
GW_TAG=v0.0.0-alpha.9f9744cf

# Never read ambient AWS_PROFILE (e.g. nsx): this script targets the account
# ATE lives in; override explicitly with MIRROR_PROFILE if that differs.
P=${MIRROR_PROFILE:-ai}

region() { echo "$ATE" | cut -d. -f4; }

auth() { # crane auth login for ECR, isolated cred store (no docker daemon)
  local acct
  acct=$(echo "$ATE" | cut -d. -f1)
  export DOCKER_CONFIG=${DOCKER_CONFIG:-/tmp/.dockercfg-mirror}
  aws --profile "$P" ecr get-login-password --region "$(region)" \
    | crane auth login "$acct.dkr.ecr.$(region).amazonaws.com" -u AWS --password-stdin
}

# ko's default namer is md5(import path): with KO_DOCKER_REPO=<base>, each
# binary publishes to <base>/<name>-<md5(import path)>:<tag> (see
# ko/pkg/publish/namer.go). Those intermediate repos must exist in ECR, then
# crane copy renames the artifact to the final <base>/<name>:<tag>.
ko_ref() { printf '%s' "$1" | md5 -q; }
create_repo() {
  aws --profile "$P" ecr create-repository --repository-name "$1" --region "$(region)" \
    >/dev/null 2>&1 || true
}

# --- substrate: ko build, default mode (--bare+tag mangles the repository) ---
SUB_MOD=github.com/agent-substrate/substrate
AX_MOD=github.com/google/ax

auth

build_and_copy() { # build_and_copy <dir> <module> <name> <tag> <final-repo>
  local dir=$1 mod=$2 name=$3 ver=$4 final=$5 interim
  interim="$ATE/$name-$(ko_ref "$mod/cmd/$name")"
  create_repo "${interim#*amazonaws.com/}"
  create_repo "${final#*amazonaws.com/}"
  (cd "$dir" && KO_DOCKER_REPO="$ATE" ko build --sbom=none -t "$ver" "./cmd/$name" >/dev/null)
  crane copy "$interim:$ver" "$final:$ver"
}

SUBSTRATE_SRC=${SUBSTRATE_SRC:-$PWD/substrate}
AX_SRC=${AX_SRC:-$PWD/ax}
SUB_TAG=${SUBSTRATE_VERSION:-a0b680d}
AX_TAG=${AX_VERSION:-c5c1ac5}

for name in ateapi atecontroller atelet podcertcontroller; do
  build_and_copy "$SUBSTRATE_SRC" "$SUB_MOD" "$name" "$SUB_TAG" "$ATE/$name"
done

for name in ax-server ax-controller ax-task-runner; do
  build_and_copy "$AX_SRC" "$AX_MOD" "$name" "$AX_TAG" "$AX/$name"
done

crane copy "$GW_REPO:$GW_TAG" "$ATE/agentgateway:$GW_TAG"
crane copy docker.io/library/redis:7-alpine "$AX/redis:7-alpine"

# Bundled infra images (chart postgres.yaml / rustfs.yaml), digest-pinned.
create_repo "${ATE#*amazonaws.com/}/postgres"
create_repo "${ATE#*amazonaws.com/}/rustfs"
create_repo "${ATE#*amazonaws.com/}/aws-cli"
crane copy "postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15" "$ATE/postgres:18-alpine"
crane copy "rustfs/rustfs:1.0.0-beta.3@sha256:378642b05b7dcb4849fb77ebe6aca4ced1c3f66e7e504247df95a5c9018d3358" "$ATE/rustfs:1.0.0-beta.3"
crane copy "amazon/aws-cli:2.17.0@sha256:643507c10ada7964ca6157b3d799f030b90577643da9955d319a77399ed80d73" "$ATE/aws-cli:2.17.0"
create_repo "${ATE#*amazonaws.com/}/otel-collector-contrib"
crane copy "otel/opentelemetry-collector-contrib:0.157.0@sha256:f2f01157055a9b2aab9df7118e1f1c9abf345e99b23bc7a2bc791db374a7d0f6" "$ATE/otel-collector-contrib:0.157.0"
create_repo "${ATE#*amazonaws.com/}/jaeger"
crane copy "jaegertracing/all-in-one:1.55@sha256:f6b5d09073f14f76873d300f565a6691d815e81bea8e07e1dc3ff67e0596dd4e" "$ATE/jaeger:1.55"

echo "mirror complete:"
crane ls "$ATE/ateapi"
