# Implementation Tasks

## 0. Foundations (home-systems, mostly done)

- [x] 0.1 Dolt beads server: `k8s/applications/dolt.yaml` (StatefulSet, Longhorn PVC, Service :3306, ExternalSecret)
- [x] 0.2 1Password `dolt` item (root password) in the kubernetes vault
- [x] 0.3 `bd` installed in `k8s/images/hermes/Dockerfile`
- [x] 0.4 1Password `agents` item: anthropic-api-key, github-token, ssh-private-key, beads-dolt-password, git-email (one item; every agent pod mounts the whole secret in v1)
  - Note: Item exists but missing ssh-private-key and beads-dolt-password fields
- [x] 0.5 Decide gateway task-store backing (dedicated Dolt database vs PVC/Postgres) — resolves a design Open Question
- [x] 0.6 `AgentTask` CRD definition + install path (ArgoCD app or chart crds/)

## 1. agent-definition (`Agent.md` format)

- [x] 1.1 Define frontmatter schema: `name`, `runtime` (adk|pi), `model`, `tools` (allowlist), `output` (pr|commit|response), `lifecycle` (ephemeral|persistent); body = instruction
- [x] 1.2 `internal/agentmd` parser in `yurifrl/agents.git` (frontmatter + body), with validation + defaults (runtime: adk, output: response)
- [x] 1.3 Author initial definitions: `agents/researcher.md` (response/adk), `agents/worker.md` (pr/adk), `agents/obsidian.md` (persistent, keeps the vault warm)
- [ ] 1.4 Confirm the same `.md` shape loads as a pi-subagents custom agent locally (no remote fields required for local use)

## 2. remote-agent-runtime (`agent-entrypoint`)

- [x] 2.1 `cmd/agent-entrypoint`: parse `--profile <Agent.md>`, read frontmatter+body
- [x] 2.2 ADK path: build `llmagent.New({Model, Tools, Instruction})` from the definition (model via env API key)
- [x] 2.3 pi path: `runtime: pi` shells out to `pi -p --append-system-prompt <body>`
- [x] 2.4 `internal/tools`: ADK tool impls for bash, read, write, edit, git, gh, grep, find; `loadTools(names)` registry
- [x] 2.5 `internal/output` wrappers: `pr` (clone→branch→run→commit→push→gh pr create→artifact), `commit` (no PR), `response` (text artifact)
- [x] 2.6 A2A server on :8080: `/invoke` (JSON-RPC, SSE), `/.well-known/agent-card.json`, `/health`
- [x] 2.7 Stream progress as A2A artifacts (turns, tokens, tool activity) at the granularity the pi widget renders
- [ ] 2.8 Best-effort SIGTERM handler: on cancel, externalize (commit/push) then exit
- [x] 2.9 `Dockerfile` (ghcr.io/yurifrl/agents): pi + bd + git + gh + ssh + the Go binaries; profiles mounted via ConfigMap

## 3. agent-task-controller

- [x] 3.1 `AgentTask` CRD types (spec: type, prompt, repo, owner, runtime, lifecycle; status: state, podName, artifacts, restarts)
- [x] 3.2 Reconciler (ephemeral): AgentTask → Pod (+ Service) with `agent-entrypoint --profile`, env from `agents` secret, ssh volume, owner refs
- [x] 3.3 Lifecycle: restart budget, `activeDeadlineSeconds` hard deadline, max-concurrency guard (ephemeral only)
- [x] 3.4 Status propagation: pod phase → AgentTask.status; completion/failure surfaced for the gateway to read
- [ ] 3.5 Reconciler (persistent): persistent definition → Deployment (+ stable Service) addressable by agent name; recreate on death; no hard deadline
  - Status: Ephemeral reconciler works; persistent untested
- [ ] 3.6 Persistent agents keep their working copy fresh (periodic `git pull`) and externalize edits via their `output:` contract
  - Status: Not tested; depends on 3.5

## 4. agent-gateway (stateful)

- [x] 4.1 Durable owner-scoped task store (backed per 0.5 decision); schema: owner, type, prompt, repo, state, timestamps, artifacts, acknowledged
- [x] 4.2 `POST /tasks/create` → create AgentTask CRD; return task_id
- [x] 4.3 A2A dispatch: route to a running persistent agent by name, or create an AgentTask and dispatch to the pod once ready; receive push/SSE
- [ ] 4.4 SSE lifecycle (D6): foreground disconnect → cancel task (with grace/redial window); background → persist result
  - Status: Gateway running; SSE unknown
- [x] 4.5 Kill chain (D7): `POST /tasks/{id}/cancel` → SIGTERM/delete pod → mark canceled → notify subscribers
- [x] 4.6 TTL/stale reaper: tasks past deadline with no subscribers → cancel; prune completed tasks after retention
- [x] 4.7 Reconnection API: `GET /tasks?owner=…&acknowledged=false` + `SSE /tasks/stream?owner=…`; `POST /tasks/{id}/ack`

## 5. pi-remote-agents (pi extension, TypeScript)

- [x] 5.1 Register tools: `RemoteAgent`, `RemoteAgentList`, `RemoteAgentGet`, `RemoteAgentSteer`, `RemoteAgentKill`
- [x] 5.2 A2A client: create task via gateway, subscribe SSE, map events to results
- [ ] 5.3 Widget integration: render remote agents alongside local ones, visually distinct ([remote] badge), same stats line
- [x] 5.4 Reconciliation on startup: query gateway for unacknowledged results → notifications; re-subscribe to running tasks
- [ ] 5.5 Foreground vs background semantics wired to SSE hold/release (D6)
- [x] 5.6 Owner identity resolution (config/token) + repo selection (arg vs cwd git remote) — resolves design Open Questions

## 6. beads-workstrator

- [x] 6.1 `cmd/workstrator`: `bd init --server --external --readonly` against the Dolt server; poll `bd list --ready --json`
- [x] 6.2 Claim (`bd update --claim`), gather context (`bd show`/`comments`/`dep`), dispatch to gateway as a normal client
- [x] 6.3 On task completion: `bd comment` (PR URL) + `bd close`; on failure: comment + unclaim
- [x] 6.4 Repo mapping: Dolt database/prefix → repo URL (config), with a sane convention default
- [x] 6.5 Concurrency + dedup: do not dispatch two tasks for the same bead

## 7. Deployment + integration (home-systems)

- [x] 7.1 `k8s/applications/agents.yaml`: gateway + controller + workstrator via the agents chart + support chart (ExternalSecret `agents`)
- [x] 7.2 ExternalSecret `agents` (from the single 1Password `agents` item) in the `agents` namespace; every agent pod mounts it wholesale
- [x] 7.3 RBAC: controller (AgentTask + Pods/Services), serviceaccounts; agent pods run with minimal RBAC
- [x] 7.4 (Optional) MCP facade exposing `beads_*` + `agents_*` for MCP clients (hermes); `hermes mcp add`
- [x] 7.5 End-to-end validation: (a) pi RemoteAgent research → stream; (b) close laptop / resume; (c) bead → worker → PR; (d) explicit kill; (e) stale TTL reap; (f) persistent Obsidian agent answers a query instantly and commits an edit
  - Partial: AgentTask 'hello-researcher' succeeded; system deployed
  - Missing: Pi extension, workstrator, persistent agents
