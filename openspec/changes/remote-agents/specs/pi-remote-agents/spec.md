## ADDED Requirements

### Requirement: Explicit remote agent tools
The pi extension SHALL expose remote-agent tools distinct from the local
`Agent()` tool: `RemoteAgent` (spawn), `RemoteAgentList`, `RemoteAgentGet`,
`RemoteAgentSteer`, and `RemoteAgentKill`. Local and remote agents SHALL NOT be
the same tool; the caller chooses deliberately.

#### Scenario: Spawn a remote agent
- **WHEN** the LLM calls `RemoteAgent({ type, prompt })`
- **THEN** the extension creates a task via the gateway and returns a task id and
  running status

#### Scenario: Local tool is unchanged
- **WHEN** the LLM calls `Agent({ … })`
- **THEN** existing local subagent behavior runs, with no remote dispatch

### Requirement: Unified but distinct widget rendering
The extension SHALL render remote agents in the same widget / `/agents` surface
as local agents, visually distinguished (e.g. a remote badge), showing
comparable live status.

#### Scenario: Remote agent appears in the widget
- **WHEN** a remote agent is running
- **THEN** it appears in the agents widget with a remote indicator and a live
  status line

### Requirement: Reconciliation on startup
On startup the extension SHALL query the gateway for the owner's unacknowledged
completed tasks (surfacing them as notifications) and re-subscribe to any still
running tasks.

#### Scenario: See results produced while away
- **WHEN** pi starts and the owner has tasks that completed while pi was closed
- **THEN** the extension surfaces those results as notifications

#### Scenario: Re-attach to running work
- **WHEN** pi starts and the owner has a task still running
- **THEN** the extension re-subscribes and shows its ongoing progress

### Requirement: Foreground/background lifecycle wired to SSE
The extension SHALL hold an SSE subscription for foreground remote tasks and
release it for background tasks, consistent with the gateway's lifecycle rules.

#### Scenario: Background remote task outlives the session
- **WHEN** a remote agent is started in background mode and pi is closed
- **THEN** the task continues and its result is retrievable on the next start
