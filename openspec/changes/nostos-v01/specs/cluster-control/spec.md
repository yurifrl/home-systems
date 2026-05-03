## ADDED Requirements

### Requirement: Bootstrap a fresh cluster
The system SHALL provide `nostos bootstrap <node>` that invokes `talosctl bootstrap` against the named controlplane node, waits for etcd to become healthy, and reports success or timeout.

#### Scenario: Happy path bootstrap
- **WHEN** `nostos bootstrap dell01` is run against a node whose apid is up and config is applied
- **THEN** `talosctl bootstrap` is invoked, the command waits up to the configured timeout (default 5 min) for etcd `Running+OK`, and exits zero on success

#### Scenario: Bootstrap on non-controlplane rejected
- **WHEN** `nostos bootstrap tp1` is run and `tp1`'s role in `config.yaml` is `worker`
- **THEN** the command exits non-zero before contacting the node with a message explaining bootstrap targets controlplane nodes only

#### Scenario: Already-bootstrapped is safe
- **WHEN** `nostos bootstrap dell01` is run against a node whose etcd is already healthy
- **THEN** the command exits zero with a message noting the cluster is already bootstrapped (idempotent)

### Requirement: Fetch kubeconfig post-bootstrap
The system SHALL fetch the cluster's kubeconfig via `talosctl kubeconfig` and write it to `state/kubeconfig` after successful bootstrap or on demand.

#### Scenario: Kubeconfig fetched after bootstrap
- **WHEN** `nostos bootstrap dell01` completes successfully
- **THEN** `state/kubeconfig` exists and contains the cluster endpoint

#### Scenario: Explicit kubeconfig refresh
- **WHEN** `nostos kubeconfig` is run against a bootstrapped cluster
- **THEN** `state/kubeconfig` is refreshed from the apiserver

### Requirement: Regenerate admin client certificate
The system SHALL provide `nostos config refresh` that generates a new Talos admin client certificate signed by the existing cluster CA, writing a fresh `state/talosconfig` without requiring an existing unexpired admin cert.

#### Scenario: Refresh with expired existing cert
- **WHEN** `nostos config refresh` is run and the existing `state/talosconfig` contains an expired client cert
- **THEN** a new keypair + CSR is generated, signed by the CA read from the rendered machineconfig (or secrets backend), and written to `state/talosconfig` — the subsequent `nostos status` call succeeds

#### Scenario: Configurable validity
- **WHEN** `nostos config refresh --hours 8760` is run
- **THEN** the resulting admin cert has `notAfter` approximately one year from now (default is 876000 hours ≈ 100 years)

#### Scenario: Admin cert is per-device
- **WHEN** `nostos config refresh` is run on two different machines using the same consumer repo
- **THEN** each machine's `state/talosconfig` contains a different admin keypair, both signed by the same CA, both working against the cluster

### Requirement: Report per-node status
The system SHALL provide `nostos status` that displays for each configured node: reachability (ping), Talos apid port 50000 state, running Talos version, kubelet health, and any stuck services.

#### Scenario: Status against healthy cluster
- **WHEN** `nostos status` is run against a running cluster
- **THEN** each node's row shows `ping: ok`, `apid: up`, `version: v1.10.3`, `kubelet: healthy`, and no stuck services

#### Scenario: Status against unreachable worker
- **WHEN** `nostos status` is run and one worker has apid down (like the bricked tp1/tp4 from this session)
- **THEN** that node's row shows `apid: refused` and a hint recommending reinstall via PXE

### Requirement: One-shot wipe flag
The system SHALL provide `nostos wipe <node>` that marks a node for a single `talos.experimental.wipe=system` boot, coordinating with the PXE serve layer to include the flag once and remove it after the node successfully reinstalls.

#### Scenario: Wipe flag added and consumed exactly once
- **WHEN** `nostos wipe dell01` marks dell01 and the node PXE-boots once
- **THEN** the served `boot.ipxe` for dell01 contains `talos.experimental.wipe=system` on exactly that one boot; after the node completes install and comes back Ready, subsequent PXE boots for dell01 do not include the flag unless `nostos wipe dell01` is run again

### Requirement: Graceful fallback when talosctl is absent
The system SHALL detect `talosctl` availability and fail with a clear message if it isn't installed; `nostos` SHALL NOT attempt to reimplement `talosctl` functionality.

#### Scenario: Missing talosctl errors cleanly
- **WHEN** any cluster-control command is run and `talosctl` is not on PATH
- **THEN** the command exits non-zero with a message explaining that `talosctl` is required and how to install it
