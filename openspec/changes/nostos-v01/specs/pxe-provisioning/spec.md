## ADDED Requirements

### Requirement: Download Talos boot assets
The system SHALL download the Talos kernel and initramfs for a configured version + schematic from `factory.talos.dev` into the local state directory.

#### Scenario: First build downloads assets
- **WHEN** `nostos build` is run and state directory is empty
- **THEN** `state/assets/vmlinuz-<arch>` and `state/assets/initramfs-<arch>.xz` exist and match the version specified in `config.yaml`

#### Scenario: Subsequent build is idempotent
- **WHEN** `nostos build` is run and assets already exist at the configured version
- **THEN** no re-download occurs and the command completes in under 1 second

#### Scenario: Version change triggers re-download
- **WHEN** the `talos_version` field in `config.yaml` changes and `nostos build` is re-run
- **THEN** the previous kernel/initramfs are replaced with the new version's assets

### Requirement: Build a custom iPXE binary
The system SHALL build an `snponly.efi` iPXE binary sized under 256 KB with an embedded chainload script that survives the consumer-router DHCP race.

#### Scenario: Build produces a valid iPXE binary
- **WHEN** `nostos build` completes successfully
- **THEN** `state/assets/ipxe.efi` exists, is under 256 KB, and contains the string `chain ${filename}` embedded in its script

#### Scenario: Missing Docker fails loud
- **WHEN** `nostos build` is run without Docker available
- **THEN** the command exits non-zero with a message explaining Docker is required and how to install it

### Requirement: Render a second-stage boot script
The system SHALL write a `boot.ipxe` script that uses iPXE's runtime `${next-server}` variable and does not hardcode the operator's Mac IP.

#### Scenario: Rendered boot.ipxe is IP-portable
- **WHEN** `nostos build` completes
- **THEN** `state/assets/boot.ipxe` references `${next-server}` and does not contain any literal IPv4 address

### Requirement: Serve HTTP + DHCP + TFTP for PXE booting
The system SHALL start an HTTP server on the configured port (default 9080) and a `dnsmasq` process providing DHCP + TFTP, filtered to PXE clients only.

#### Scenario: Serve starts both subprocesses
- **WHEN** `nostos serve` is run
- **THEN** an HTTP server is listening on the configured port serving `state/assets/` AND a dnsmasq process is running bound to the detected ethernet interface

#### Scenario: Serve does not interfere with non-PXE DHCP clients
- **WHEN** `nostos serve` is running and a non-PXE device on the LAN sends DHCPDISCOVER
- **THEN** nostos's dnsmasq does not respond (vendor-class filtering), leaving the consumer's existing DHCP server authoritative

#### Scenario: Stale processes cleaned on start
- **WHEN** `nostos serve` is run while a previous HTTP server still holds port 9080
- **THEN** the stale process is terminated and the new server binds successfully

### Requirement: Handle wipe-on-reinstall cases
The system SHALL provide a mechanism to boot a node with `talos.experimental.wipe=system` for exactly one boot, preventing the infinite wipe-loop trap when BIOS defaults to PXE-first.

#### Scenario: Wipe flag is one-shot
- **WHEN** `nostos wipe <node>` marks a node for wipe and serves a PXE boot with the wipe flag
- **THEN** after the node successfully re-installs and reboots, the next PXE serve no longer includes the wipe flag for that node without an explicit re-request
