## Why

Today the only way to run pi work is on a machine you are sitting at: local
subagents (`@tintinweb/pi-subagents`) spawn child processes that share your
filesystem and die with your session. That is perfect for "think alongside me"
helpers, but it cannot do the thing we actually want: hand off a unit of work to
a remote, ephemeral agent, close the laptop, and pick the result up tomorrow.

This change introduces a **second, distinct class of agent** — a *remote agent*
— with its own lifecycle, its own filesystem (an ephemeral pod), and a durable
result contract. Remote agents are declared Kubernetes-style: an `Agent.md`
definition is to a running agent what a Pod spec is to a container. The generic
ADK runner has no personality of its own; the `Agent.md` gives it one. The same
`Agent.md` can also drive a `pi` runtime when we want remote interactive access.

The headline requirement that shapes the whole design: **reconciliation**. A
user must be able to spawn remote agents, disconnect entirely, and on
reconnection see what finished while they were gone and re-attach to what is
still running. Ownership therefore survives pi restarts; it is not tied to a pi
session.

This is decoupled from any specific caller. hermes is **one example** of an
external client that can talk to the system (over A2A or MCP); the system itself
knows nothing about hermes.

## What Changes

- **New agent class — remote agents.** Distinct from local subagents in state
  model (isolated pod vs shared filesystem), output model (durable artifact /
  streamed response vs in-place side effects), lifecycle (independent vs coupled
  to the session), and failure mode (lost work vs lost nothing). Treated as
  stateless: clone → work → externalize result → die; a death before
  externalizing means the task is simply retried.

- **Two remote lifecycles — ephemeral and persistent.** An `Agent.md` declares
  `lifecycle: ephemeral` (default — spawned per task, dies when done) or
  `lifecycle: persistent` (always-on). A persistent agent (e.g. an Obsidian
  agent that keeps its vault cloned and warm) is reconciled into a Deployment
  with a stable A2A endpoint, so queries and edits hit it instantly with no
  cold-start. The gateway routes to a persistent agent by name; it spawns an
  ephemeral pod per task otherwise.

- **`Agent.md` as a declarative agent definition (Kubernetes-style).** YAML
  frontmatter (`name`, `runtime`, `model`, `tools`, `output`) plus a markdown
  body (the system prompt / ADK instruction). The same file is consumed two
  ways: locally by pi-subagents, remotely by the `agent-entrypoint` which builds
  an ADK agent (or invokes `pi`) from it. `output:` is a behavior contract, not
  mere metadata — `pr` wraps the run in clone/branch/commit/push/open-PR,
  `response` returns the agent's text via A2A, `commit` pushes a branch without a
  PR.

- **Two pi-facing workflows, one shape.** *Query* (research → streamed answer)
  and *Execute* (same, but with side effects) are the same interaction from pi's
  side: send a prompt, stream a result. The difference lives entirely in the
  `Agent.md` definition, not in pi.

- **Explicit remote tools (not transparent `Agent()` routing).** pi gets new
  tools — `RemoteAgent`, `RemoteAgentList`, `RemoteAgentGet`, `RemoteAgentSteer`,
  `RemoteAgentKill` — so pi (and the LLM) *chooses* local vs remote deliberately.
  Local and remote are not interchangeable; their control semantics differ.

- **Stateful A2A gateway** as the system's front door: owner-scoped task store
  that survives pi restarts, SSE connection lifecycle (foreground task cancels on
  disconnect, background task persists), kill propagation (gateway → pod), TTL /
  stale-pod reaping, and a reconnection API (`GET /tasks?owner=…&acknowledged=false`
  plus an SSE stream) that powers reconciliation.

- **`AgentTask` CRD + controller.** The gateway creates an `AgentTask`; a
  controller reconciles it into a Pod (+ Service) running `agent-entrypoint` with
  the selected `Agent.md`, owner references for GC, restart budget, and
  `activeDeadlineSeconds` as the hard stale ceiling.

- **`workstrator`** (beads front): polls the Dolt beads server for ready beads,
  claims them, gathers context, and dispatches to the gateway exactly like any
  other client — closing the bead when the task completes.

- **Deployment glue in home-systems** (already begun): the Dolt beads server
  (`k8s/applications/dolt.yaml`, 1Password `dolt` item) and a future
  `k8s/applications/agents.yaml` for the gateway + controller + workstrator.
  `bd` is already installed in the hermes image so hermes can talk beads.

Out of scope: replacing local subagents (they stay as-is); a specific hermes
integration beyond "hermes is an A2A/MCP client like any other"; multi-user
governance / budgets; a web UI.

## Capabilities

### New Capabilities
- `agent-definition`: The `Agent.md` declarative format and its dual
  consumption — frontmatter schema (`runtime`, `model`, `tools`, `output`), body
  as system prompt, and the contract that the same file yields a local pi
  subagent or a remote ADK/pi instance.
- `remote-agent-runtime`: The `agent-entrypoint` that turns an `Agent.md` into a
  running A2A server inside a pod — building an ADK agent (or invoking `pi`),
  registering the declared tools, applying the `output:` behavior wrapper, and
  streaming progress as A2A artifacts.
- `agent-gateway`: The stateful A2A gateway — owner-scoped durable task store,
  SSE lifecycle semantics, kill propagation, TTL reaping, and the reconnection
  API that makes "close laptop, resume tomorrow" work.
- `agent-task-controller`: The `AgentTask` CRD and its reconciler — ephemeral
  task → Pod (+ Service), and persistent agent → Deployment (+ Service) with a
  stable endpoint — owner references, restart budget, hard deadline (ephemeral).
- `pi-remote-agents`: The pi extension exposing the explicit remote tools and
  reconciliation behavior, rendering remote agents in the same widget /
  `/agents` surface as local ones but visually distinct.
- `beads-workstrator`: The beads-front poller that claims ready beads and
  dispatches them to the gateway as a normal client, then closes them.

### Modified Capabilities
- None in home-systems beyond additive deployment manifests; the bulk of code
  lives in the separate `yurifrl/agents.git` repo.

## Impact

- **New repo `yurifrl/agents.git`** (the system): `cmd/agent-entrypoint`,
  `cmd/gateway`, `cmd/workstrator`, `cmd/controller` (or controller-runtime
  manager), `internal/agentmd` (parse `Agent.md`), `internal/tools` (ADK tool
  impls: bash/read/write/edit/git/gh/grep/find), `internal/output` (pr/response/
  commit wrappers), `internal/a2a`, `agents/*.md` definitions, `Dockerfile`,
  `chart/`, and the pi extension (`pi-remote-agents`, TypeScript).
- **home-systems (this repo):**
  - Done: `k8s/applications/dolt.yaml` (Dolt beads server, Longhorn PVC,
    VirtualService), 1Password `dolt` item, `bd` added to
    `k8s/images/hermes/Dockerfile`.
  - New: `k8s/applications/agents.yaml` (gateway + controller + workstrator via
    the agents chart + support chart), a 1Password `agents` item (Anthropic key,
    GitHub token, SSH key, Dolt password, git email) surfaced as an
    ExternalSecret, and the `AgentTask` CRD install.
- **Secrets:** one 1Password `agents` item (Anthropic key, GitHub token, SSH
  key, Dolt password, git email) surfaced as a single ExternalSecret in the
  `agents` namespace. Every agent pod mounts the whole secret for now; no
  per-agent scoping in v1.
- **No impact** on existing apps; everything here is additive.
