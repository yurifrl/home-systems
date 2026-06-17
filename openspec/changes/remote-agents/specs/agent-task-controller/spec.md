## ADDED Requirements

### Requirement: AgentTask CRD reconciled into a pod
The system SHALL define an `AgentTask` custom resource and a controller that
reconciles each `AgentTask` into a Pod (and Service) running `agent-entrypoint`
with the selected `Agent.md`, the required secrets, and an SSH volume for git.
The Pod SHALL be owned by the `AgentTask` for garbage collection.

#### Scenario: Task creates a pod
- **WHEN** an `AgentTask` is created
- **THEN** the controller creates a Pod running `agent-entrypoint` for the named
  agent definition

#### Scenario: Deleting the task removes the pod
- **WHEN** an `AgentTask` is deleted
- **THEN** its owned Pod and Service are garbage-collected

### Requirement: Lifecycle controls
The controller SHALL enforce a restart budget, a hard `activeDeadlineSeconds`
ceiling, and a max-concurrency limit on agent pods.

#### Scenario: Restart budget exhausted
- **WHEN** a pod fails more times than the restart budget allows
- **THEN** the controller stops recreating it and marks the AgentTask failed

#### Scenario: Hard deadline terminates a pod
- **WHEN** a pod runs past its deadline
- **THEN** it is terminated regardless of state

#### Scenario: Max concurrency respected
- **WHEN** the number of active agent pods is at the configured limit
- **THEN** new AgentTasks wait rather than spawning additional pods

### Requirement: Status propagation
The controller SHALL reflect pod phase into `AgentTask.status` (state, pod name,
restarts, artifacts) so the gateway can observe completion and failure.

#### Scenario: Completion is observable
- **WHEN** a pod completes successfully
- **THEN** the AgentTask status transitions to completed with its artifacts

### Requirement: Persistent agents reconciled into a Deployment
For an agent definition declaring `lifecycle: persistent`, the controller SHALL
reconcile a Deployment (and Service) with a stable in-cluster endpoint that stays
running, rather than a per-task pod. The endpoint SHALL be addressable by the
agent name so the gateway can route to it directly.

#### Scenario: Persistent agent stays running
- **WHEN** a persistent agent definition is applied
- **THEN** the controller maintains an always-on Deployment serving the agent's
  A2A endpoint, recreating it if it dies

#### Scenario: Gateway routes to a running persistent agent
- **WHEN** a client targets a persistent agent by name
- **THEN** the gateway sends the task to the existing endpoint without spawning a
  new pod

#### Scenario: Persistent agent has no hard deadline
- **WHEN** a persistent agent runs for an extended period
- **THEN** it is not subject to the ephemeral hard-deadline reaping
