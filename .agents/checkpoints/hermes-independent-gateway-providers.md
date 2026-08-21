---
created: 2026-08-16
project: home-systems
description: Configure Hermes with independent LiteLLM and OmniRoute providers, repair OmniRoute health, and refresh dashboard catalogs.
session_id: 01a00a12-81fd-7ffb-b479-f45b955ac4ca
resume_with: cly agent-session resume --provider pi hermes-independent-gateway-providers
checkpoint_file: .agents/checkpoints/hermes-independent-gateway-providers.md
---

## Context
Hermes needs LiteLLM and OmniRoute as separate named custom OpenAI-compatible providers, rather than routing one gateway through the other. Hermes dashboard is at `https://hermes.syscd.live/models`.

## Decisions
- Hermes `providers` has independent `litellm` and `omniroute` endpoints, selected as `custom:litellm:<model>` and `custom:omniroute:<model>`.
- Removed the global `OPENAI_BASE_URL` default and did not proxy OmniRoute via LiteLLM.
- OmniRoute NetworkPolicy is egress-only. Ambient/CNI source identity made an ingress allowlist block kubelet pod-IP health probes despite the app being locally healthy.
- Created a dedicated OmniRoute `chat` API key for Hermes. `API_KEY_SECRET` is an internal signing secret, not an API client credential.

## Current State
- Hermes config includes `providers.litellm` at `http://litellm.litellm.svc.cluster.local:4000/v1` and `providers.omniroute` at `http://omniroute.omniroute.svc.cluster.local:20128/v1`.
- `hermes-env` in 1Password has `LITELLM_API_KEY` and the dedicated `OMNIROUTE_API_KEY`.
- OmniRoute is Ready and ArgoCD Synced/Healthy; its egress-only policy permits normal kubelet and service ingress.
- Hermes was restarted after External Secrets refreshed; the live Hermes pod retrieves 1,426 OmniRoute models.
- Dashboard model refresh completed: OmniRoute shows 1,426 models and LiteLLM shows 707 models.

## Lessons
- Hermes `envFrom` keys require a pod rollout after an ExternalSecret changes.
- ArgoCD Application has `ServerSideApply=true`; do not request force sync together with server-side apply.
- When an existing NetworkPolicy retains removed fields after reconciliation, delete only the GitOps-managed policy with explicit approval, then let ArgoCD recreate it.

## Next Steps
- No required follow-up. Users can choose models from the Hermes dashboard or with `custom:litellm:<model>` and `custom:omniroute:<model>`.
