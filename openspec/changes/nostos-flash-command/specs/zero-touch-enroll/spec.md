## ADDED Requirements

### Requirement: Node auto-joins cluster via Tailscale
A node booted from a ship-produced image SHALL automatically connect to the tailnet, accept routes, reach the cluster endpoint, and join etcd without operator intervention.

#### Scenario: Remote node powers on
- **WHEN** a shipped node boots from the image at a remote site with internet access
- **THEN** Tailscale extension starts and authenticates with the pre-minted key
- **AND** node accepts advertised routes from existing cluster nodes
- **AND** etcd joins the existing cluster as a learner
- **AND** node appears as Ready in `kubectl get nodes` within 5 minutes

#### Scenario: No internet at boot
- **WHEN** a shipped node boots without internet access
- **THEN** Tailscale extension retries connection with exponential backoff
- **AND** node joins the cluster once internet becomes available

### Requirement: All nodes accept Tailscale routes by default
All node templates SHALL include `TS_EXTRA_ARGS=--accept-routes` in the Tailscale extension configuration to enable cross-subnet mesh routing.

#### Scenario: New node config includes accept-routes
- **WHEN** operator runs `nostos render <node>` for any node
- **THEN** the rendered machineconfig includes `TS_EXTRA_ARGS=--accept-routes` in the Tailscale extension environment

#### Scenario: Existing node receives accept-routes on next apply
- **WHEN** operator updates a node template to include accept-routes and runs `nostos apply <node>`
- **THEN** the Tailscale extension restarts with route acceptance enabled
- **AND** the node can reach subnets advertised by other tailnet peers

### Requirement: Cluster endpoint reachable via tailnet
The shipped node's machineconfig SHALL reference the cluster API endpoint in a way that is reachable once Tailscale connects (via subnet routing from the existing controlplane).

#### Scenario: etcd peer communication across subnets
- **WHEN** rpi01 (192.168.0.x) joins the cluster with dell01 (192.168.68.x)
- **THEN** etcd peers communicate via Tailscale subnet routing
- **AND** both nodes have accept-routes enabled
- **AND** both subnets are advertised and approved in Tailscale admin
