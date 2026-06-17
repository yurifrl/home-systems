## ADDED Requirements

### Requirement: Poll and claim ready beads
The workstrator SHALL connect to the Dolt beads server in external read-capable
mode, poll for ready beads, and atomically claim a bead before dispatching work
for it.

#### Scenario: Ready bead is claimed
- **WHEN** the workstrator finds a ready bead
- **THEN** it claims the bead before creating any task for it

#### Scenario: No double dispatch
- **WHEN** a bead is already claimed/in-progress
- **THEN** the workstrator does not dispatch a second task for it

### Requirement: Dispatch as a normal gateway client
The workstrator SHALL gather a bead's context (description, comments,
dependencies) and dispatch it to the gateway using the same client interface as
any other caller. The system SHALL NOT special-case the workstrator.

#### Scenario: Context-rich dispatch
- **WHEN** the workstrator dispatches a claimed bead
- **THEN** the task includes the bead's title, description, and relevant context

### Requirement: Close the loop on completion
On task completion the workstrator SHALL comment the result (e.g. a PR URL) on
the bead and close it; on failure it SHALL comment the error and release the
claim.

#### Scenario: Completed task closes the bead
- **WHEN** a dispatched task completes with a PR artifact
- **THEN** the workstrator comments the PR URL and closes the bead

#### Scenario: Failed task releases the bead
- **WHEN** a dispatched task fails
- **THEN** the workstrator comments the failure and unclaims the bead so it can
  be retried

### Requirement: Repo resolution per bead
The workstrator SHALL resolve which repository a bead targets from its Dolt
database/prefix via configuration, with a convention-based default.

#### Scenario: Database maps to repo
- **WHEN** a bead comes from a known Dolt database/prefix
- **THEN** the workstrator dispatches the task against the mapped repository
