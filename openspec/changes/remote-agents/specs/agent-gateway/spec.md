## ADDED Requirements

### Requirement: Owner-scoped durable task store
The gateway SHALL persist each task keyed by an `owner` that survives client
restarts (not by ephemeral session). Each record SHALL include owner, agent
type, prompt, repo, state, timestamps, artifacts, and an `acknowledged` flag.
The store SHALL survive a gateway restart.

#### Scenario: Task survives client disconnect
- **WHEN** a client creates a task and then disconnects entirely
- **THEN** the task record persists and remains queryable by its owner

#### Scenario: Store survives gateway restart
- **WHEN** the gateway process restarts
- **THEN** previously recorded tasks are still retrievable by owner

### Requirement: Reconnection API
The gateway SHALL expose a way to retrieve an owner's unacknowledged completed
tasks and to subscribe to live updates for that owner's tasks, plus a way to
acknowledge a task.

#### Scenario: Resume after being away
- **WHEN** a client reconnects and queries unacknowledged tasks for its owner
- **THEN** the gateway returns tasks that completed while the client was absent

#### Scenario: Re-attach to running task
- **WHEN** a client reconnects while one of its tasks is still running
- **THEN** the client can re-subscribe and receive ongoing progress updates

#### Scenario: Acknowledged tasks are not re-reported
- **WHEN** a client acknowledges a completed task
- **THEN** subsequent unacknowledged queries for that owner omit it

### Requirement: SSE lifecycle governs foreground vs background death
The gateway SHALL treat a held SSE subscription as a foreground task and an
absent subscription as a background task. On foreground subscriber disconnect the
gateway SHALL cancel the task (after a short grace/redial window); a background
task SHALL run to completion and persist its result.

#### Scenario: Foreground task cancels on disconnect
- **WHEN** the sole SSE subscriber of a foreground task disconnects and does not
  return within the grace window
- **THEN** the gateway cancels the task and kills its pod

#### Scenario: Background task survives disconnect
- **WHEN** a background task has no active subscriber
- **THEN** it continues to completion and its result is stored for later retrieval

### Requirement: Kill propagation with TTL backstop
The gateway SHALL propagate an explicit cancel to the pod (SIGTERM with a grace
period, then deletion) and mark the task canceled. Independently, every task's
pod SHALL carry a hard deadline so an unreachable or abandoned pod always
terminates.

#### Scenario: Explicit kill stops the pod
- **WHEN** a client cancels a running task
- **THEN** the gateway signals the pod to stop and marks the task canceled,
  notifying subscribers

#### Scenario: Abandoned pod is reaped
- **WHEN** a pod exceeds its hard deadline with no active subscriber
- **THEN** it is terminated and the task is marked failed/canceled
