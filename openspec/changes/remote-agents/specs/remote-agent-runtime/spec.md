## ADDED Requirements

### Requirement: Agent.md becomes a running A2A server
The `agent-entrypoint` SHALL read an `Agent.md`, build the agent (ADK or pi per
`runtime`), and expose it over A2A on a known port, serving `/invoke` (JSON-RPC
2.0 with SSE streaming), `/.well-known/agent-card.json`, and `/health`.

#### Scenario: Serves an agent card
- **WHEN** `agent-entrypoint` has started for a definition
- **THEN** `GET /.well-known/agent-card.json` returns a card naming the agent and
  its declared skills

#### Scenario: Executes a task over A2A
- **WHEN** a client sends an A2A `message/send` to `/invoke` with a prompt
- **THEN** the agent runs and streams progress, ending in a completed task with
  artifacts

### Requirement: Output behavior wrapper
The runtime SHALL apply setup/teardown based on the definition's `output:` field.
For `pr`: clone the target repo, create a branch, run the agent, commit, push,
open a PR, and return the PR URL as an artifact. For `commit`: same without
opening a PR. For `response`: no git setup; the agent's final text is the
artifact.

#### Scenario: PR output opens a pull request
- **WHEN** a definition with `output: pr` completes its work against a repo
- **THEN** the runtime pushes a branch and opens a PR, returning the PR URL as an
  artifact

#### Scenario: Response output returns text
- **WHEN** a definition with `output: response` completes
- **THEN** the runtime returns the agent's final text as the artifact with no git
  operations

#### Scenario: Commit output pushes without a PR
- **WHEN** a definition with `output: commit` completes
- **THEN** the runtime pushes the branch and returns the branch ref, opening no PR

### Requirement: Stateless execution with retry on loss
The runtime SHALL treat the pod as disposable: work not externalized as an
artifact is lost when the pod dies, and such a task SHALL be retried from a fresh
start rather than resumed mid-run. The system SHALL NOT require mid-run
checkpointing.

#### Scenario: Death before result triggers retry
- **WHEN** a pod dies before producing its artifact
- **THEN** the task is marked failed and is eligible for retry with a fresh clone

### Requirement: Streamed progress at widget granularity
The runtime SHALL stream progress as A2A artifacts carrying enough detail (turn
count, token usage, current tool activity) for a client to render a live status
comparable to local subagents.

#### Scenario: Tool activity is visible mid-run
- **WHEN** the agent invokes a tool during a run
- **THEN** an artifact describing that activity is streamed to subscribers before
  completion
