## ADDED Requirements

### Requirement: Declarative Agent.md format
The system SHALL define an agent via an `Agent.md` file: YAML frontmatter plus a
markdown body. Frontmatter SHALL support `name`, `runtime` (`adk` | `pi`,
default `adk`), `model`, `tools` (an allowlist), `output` (`pr` | `commit` |
`response`, default `response`), and `lifecycle` (`ephemeral` | `persistent`,
default `ephemeral`). The markdown body SHALL be the agent's system
prompt / instruction. The generic runner SHALL have no built-in personality; all
personality SHALL come from the definition.

#### Scenario: Minimal definition defaults
- **WHEN** an `Agent.md` declares only `name` and a body
- **THEN** it loads with `runtime: adk`, `output: response`, and the body as its
  instruction

#### Scenario: Tools are an allowlist
- **WHEN** a definition sets `tools: [read, grep, find]`
- **THEN** the resulting agent has only those tools available and no others

#### Scenario: Invalid output value rejected
- **WHEN** a definition sets `output: deploy` (not pr|commit|response)
- **THEN** parsing fails with a validation error before any agent is built

#### Scenario: Persistent lifecycle declared
- **WHEN** a definition sets `lifecycle: persistent`
- **THEN** it loads as a persistent agent (reconciled into an always-on
  Deployment) rather than an ephemeral per-task pod

#### Scenario: Lifecycle defaults to ephemeral
- **WHEN** a definition omits `lifecycle`
- **THEN** it loads as an ephemeral agent spawned per task

### Requirement: Dual consumption (local and remote)
The same `Agent.md` SHALL be consumable by the local pi runtime (as a
pi-subagents custom agent) and by the remote runtime (`agent-entrypoint`)
without modification. A definition usable remotely SHALL remain usable locally.

#### Scenario: Remote build from definition
- **WHEN** `agent-entrypoint` is given an `Agent.md` with `runtime: adk`
- **THEN** it builds an ADK agent using the definition's model, tools, and body
  as instruction

#### Scenario: pi runtime selection
- **WHEN** a definition sets `runtime: pi`
- **THEN** `agent-entrypoint` invokes `pi` with the body appended to the system
  prompt instead of building an ADK agent

#### Scenario: Local use needs no remote fields
- **WHEN** the same definition is loaded locally by pi-subagents
- **THEN** it functions as a local custom agent and ignores remote-only concerns
