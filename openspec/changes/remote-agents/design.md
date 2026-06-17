## Context

We have a Dolt-backed beads server reachable in-cluster (`dolt.dolt.svc:3306`),
`bd` understands external/server mode (`bd init --server --external
--server-host … --readonly`, password via `BEADS_DOLT_PASSWORD`), and the hermes
image already ships `bd`. Local subagents exist via `@tintinweb/pi-subagents`
(child processes, shared fs, die with the session, rich widget UI, `Agent` /
`steer_subagent` / `get_subagent_result`).

The reference architecture we studied is NSX **Argus** (`/Users/yuri/Workdir/Nsx/argus`):
a Go platform that runs agents as K8s-native resources. Key transferable
patterns:
- `argus-adk-go` builds agents from config via **Google ADK**
  (`google.golang.org/adk`): `llmagent.New({Model, Tools, Instruction})` — the
  runner is generic, personality comes from config. (`internal/agent/factory.go`,
  `internal/agent/model.go`)
- A2A via `github.com/a2aproject/a2a-go`: `a2asrv.NewHandler`, `/invoke`,
  `/.well-known/agent-card.json`, SSE streaming, **push notifications** to a
  caller webhook. (`argus-adk-go/internal/server/server.go`)
- A CRD controller reconciles `SandboxSession` → ephemeral Pod with TTL, restart
  budget, owner refs, max-concurrency. (`argus-core/internal/controller/sandbox/`)
- `spawn_tools.go` shows agent→agent delegation with per-session locks, dead
  worker recovery, and status push — the exact lifecycle hardening we need.

The decisive constraint is **reconciliation**: ownership must outlive a pi
session so the user can disconnect and resume. That rules out session-scoped
state and forces an owner-scoped, durable task store in the gateway.

## Goals / Non-Goals

**Goals**
- A second agent class (remote) that is *explicitly* different from local
  subagents, with its own pi tools and control semantics.
- `Agent.md` as the single, portable, declarative definition consumed by both
  the local (pi) and remote (ADK/pi) runtimes.
- Stateless task execution with a durable *result* (PR / pushed branch /
  streamed response) and trivial retry on death-before-result.
- Reliable reconciliation: list-what-finished + re-attach-to-running, keyed by
  owner, across pi restarts.
- Robust kill/stale handling: explicit kill propagates to the pod; abandoned
  pods are reaped by TTL.
- Caller-agnostic: pi extension, a beads workstrator, and external clients
  (hermes, curl) all hit the same A2A/MCP surface.

**Non-Goals**
- Replacing or modifying local subagents.
- A bespoke hermes integration (hermes is just an A2A/MCP client).
- Budgets, multi-tenant governance, web UI, org charts (Argus has these; we do
  not need them).
- Stateful resumable *execution* (we resume *results*, not mid-run agent state).

## Decisions

### D1 — Remote agents are a distinct class, with distinct pi tools
Local `Agent()` stays untouched. Remote work uses new tools (`RemoteAgent`,
`RemoteAgentList`, `RemoteAgentGet`, `RemoteAgentSteer`, `RemoteAgentKill`). The
LLM chooses local vs remote on purpose. Rationale: they differ in state, output,
lifecycle, and failure semantics — hiding that behind one tool would mislead the
model and the user.

### D2 — `Agent.md` is the declarative unit; the ADK runner is generic
Frontmatter: `name`, `runtime` (`adk` | `pi`), `model`, `tools` (allowlist),
`output` (`pr` | `commit` | `response`). Body = system prompt / instruction.
`agent-entrypoint` parses it and either builds an ADK `llmagent` (registering the
named tools from `internal/tools`) or invokes `pi -p --append-system-prompt`.
Locally, pi-subagents already reads the same `.md` shape. Rationale: one source
of truth, Kubernetes-style declaration, no priming baked into the runner.

### D3 — `output:` is a behavior wrapper, not metadata
`agent-entrypoint` does setup/teardown around the agent run based on `output:`:
- `pr`: clone repo → `git checkout -b pi/<slug>` → run → add/commit/push → `gh pr
  create` → return PR URL as artifact.
- `commit`: same minus the PR.
- `response`: no git; the agent's final text is the artifact.
Rationale: "always open a PR" is behavior the definition declares, not logic the
model must remember.

### D4 — Stateless execution, durable result, retry on loss
The pod is a dead-man-walking: anything not externalized vanishes. We do **not**
build mid-run checkpointing in v1. If a pod dies before producing its result,
the task is marked failed and is retried (a fresh clone). Rationale: matches the
user's "think of it as stateless" framing; checkpointing is a later option, not
a v1 requirement.

### D5 — Gateway owns durable, owner-scoped task state
The gateway is **stateful**, not a dumb proxy. Each task records `owner`,
`agent_type`, `prompt`, `repo`, `state`, timestamps, `artifacts`,
`acknowledged`, and active SSE subscribers. Owner (e.g. `yuri`) — not pi session
— is the durable key. Reconnection API: `GET /tasks?owner=…&acknowledged=false`
for "what finished while I was gone" + an SSE stream for live updates. Rationale:
this is the only way "close laptop, resume tomorrow" works.

### D6 — SSE lifecycle decides foreground vs background death
Foreground remote task: pi holds an SSE stream; if it drops (pi closed/crashed),
the gateway cancels the task → kills the pod. Background remote task: pi
disconnects immediately; the pod runs to completion and the result persists for
later retrieval. Rationale: mirrors local subagent foreground/background
expectations while respecting that the pod is not owned by pi.

### D7 — Kill chain: gateway → pod, with a hard TTL backstop
`RemoteAgentKill` → `POST /tasks/{id}/cancel` → gateway sends SIGTERM (grace
period) to the pod (or deletes it); pod does best-effort externalize-then-exit;
gateway marks `canceled` and notifies subscribers. Independently, every pod has
`activeDeadlineSeconds` so an abandoned/unreachable pod always dies. Rationale:
network can fail; the TTL guarantees no immortal pods.

### D8 — CRD controller, not direct pod creation
The gateway creates an `AgentTask` CRD; a controller reconciles it into Pod +
Service with owner refs (GC), restart budget, and the deadline. Rationale:
self-healing + declarative + free GC, exactly as Argus does with
`SandboxSession`; avoids the gateway hand-managing pod lifecycles.

### D9 — Runtime is selectable per definition (`adk` | `pi`)
`runtime: adk` → fast, in-Go agent loop for automated work. `runtime: pi` → full
pi ecosystem, and the door to remote *interactive* pi (attach / remote
subagent). Rationale: the user wants both — ADK for cheap automated runs, pi when
they want to remote into a full pi.

### D10 — System is caller-agnostic; consumers attach via A2A or MCP
The gateway speaks A2A. An MCP facade can expose `beads_*` and `agents_*` tools
for MCP-only clients (hermes, pi). The workstrator is just another client.
Nothing in the system references hermes. Rationale: the user was explicit — the
system must not know about hermes.

### D11 — Lifecycle is per definition (`ephemeral` | `persistent`)
`lifecycle: ephemeral` (default) reconciles a task into a Pod that dies when
done. `lifecycle: persistent` reconciles the agent into a Deployment with a
stable Service endpoint that stays warm (repo cloned, context loaded), so
queries/edits hit it with no cold-start — e.g. an always-on Obsidian agent. The
gateway routes to a persistent agent by name (it is already running) and spawns
an ephemeral pod otherwise. Persistent agents keep their working copy fresh
(periodic `git pull`) and still externalize edits via their `output:` contract.
Rationale: the user wants an always-available, fast-to-query agent alongside the
fire-and-forget workers; mirrors Argus's Agent-Deployment vs SandboxSession-Pod
split.

### D12 — One secret for all agents (v1)
A single 1Password `agents` item becomes one Kubernetes Secret; every agent pod
mounts the whole thing (Anthropic key, GitHub token, SSH key, Dolt password, git
email). No per-agent secret scoping yet. Rationale: keep v1 simple; scoping can
come later without changing the agent contract.

## Architecture

```
 pi (laptop)                         external clients (hermes, curl, …)
  │  RemoteAgent()/List/Get/Steer/Kill   │  A2A or MCP
  │  (pi-remote-agents extension)        │
  └───────────────┬──────────────────────┘
                  ▼
         ┌─────────────────────────────────────────────┐
         │  agent-gateway (stateful)                    │
         │   owner-scoped task store (durable)          │
         │   SSE lifecycle · kill · TTL reap            │
         │   reconnection API                           │
         └───────┬─────────────────────────┬────────────┘
        creates  │ AgentTask CRD            │ A2A /invoke + SSE
                 ▼                          ▼
      ┌────────────────────┐     ┌──────────────────────────┐
      │ agent-task         │     │ agent pod (ephemeral)     │
      │ controller         │────▶│  agent-entrypoint         │
      │ Pod+Svc, GC, TTL,  │ Pod │   reads Agent.md          │
      │ restart budget     │     │   build ADK | invoke pi   │
      └────────────────────┘     │   output: pr|commit|resp  │
                                 │   A2A server :8080 (SSE)  │
                                 └──────────────────────────┘

  workstrator ──poll/claim/close──▶ Dolt beads server (dolt.dolt.svc:3306)
       │ dispatch (as a normal client)
       └────────────────────────────▶ agent-gateway
```

## Risks / Trade-offs

- **No mid-run checkpointing (D4):** a long task killed near the end loses all
  work and retries from scratch. Accepted for v1; `output: commit`-often is a
  future mitigation.
- **Gateway is a single stateful choke point (D5):** its task store must be
  persisted (PVC or a Dolt/DB table) or a gateway restart loses reconciliation
  state. Decision: back the task store with the same Dolt server (a dedicated
  database) so it survives restarts and we run no extra datastore.
- **Foreground-cancel-on-disconnect (D6)** can kill a task on a flaky network
  blip. Mitigation: a short grace/redial window before the gateway cancels.
- **Re-implementing pi tools in ADK (`internal/tools`)** risks behavior drift
  from real pi. Mitigation: `runtime: pi` exists for fidelity; ADK tools cover
  the common bash/read/write/edit/git/gh surface only.
- **Two repos** (system in `yurifrl/agents.git`, deploy manifests in
  home-systems) means cross-repo coordination. Accepted; mirrors the existing
  board-games / home-systems-values split.

## Migration Plan

1. Land the Dolt beads server + 1Password `agents` item + `AgentTask` CRD
   (additive, no consumers yet).
2. Ship `agent-entrypoint` + one `Agent.md` (`response` runtime: adk) and prove
   a single A2A `/invoke` round-trip in a pod.
3. Add the controller (AgentTask → Pod) and the stateful gateway (Dolt-backed
   task store + reconnection API).
4. Ship the `pi-remote-agents` extension; validate close-laptop / resume.
5. Add `output: pr` and a `worker` definition; validate end-to-end bead → PR.
6. Add the `workstrator`; validate autonomous bead pickup.
7. (Optional) MCP facade for hermes/other clients.

## Open Questions

- Task-store backing: confirm a dedicated Dolt database vs a small Postgres/PVC.
  (Leaning Dolt to avoid new infra.)
- Owner identity: how is `owner` established for pi (config value? token?) so two
  laptops as the same user share a view — and is that even desired?
- Repo selection for `RemoteAgent()` from pi: explicit arg, or inferred from the
  pi cwd's git remote?
- Do we want remote *interactive* pi (`runtime: pi`, keep-alive, attach) in v1 or
  defer until the automated path is solid?
