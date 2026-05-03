## ADDED Requirements

### Requirement: Download Talos boot assets
The system SHALL download the Talos kernel and initramfs for the configured version + schematic from `factory.talos.dev` into `<state-dir>/assets/`.

#### Scenario: First build downloads assets
- **WHEN** `nostos build` runs and state is empty
- **THEN** `state/assets/vmlinuz-<arch>` and `state/assets/initramfs-<arch>.xz` exist and match the version in `config.yaml`

#### Scenario: Subsequent build is idempotent
- **WHEN** `nostos build` runs and assets already exist at the configured version
- **THEN** no re-download occurs and the command completes in under 1 second

#### Scenario: Version change triggers re-download
- **WHEN** `talos_version` in `config.yaml` changes and `nostos build` re-runs
- **THEN** the new kernel/initramfs replace the previous version's files

### Requirement: Build a custom iPXE binary
The system SHALL build an `snponly.efi` iPXE binary sized under 300 KiB, with an embedded chainload script that tolerates router DHCP races.

#### Scenario: Build produces a valid small iPXE binary
- **WHEN** `nostos build` completes successfully
- **THEN** `state/assets/ipxe.efi` exists, is under 300 KiB, and its embedded script contains `chain ${filename}` gated by a DHCP retry loop with `isset ${filename}`

#### Scenario: Missing Docker fails loud
- **WHEN** `nostos build` runs without Docker available
- **THEN** the command exits non-zero with a message explaining Docker is required for v0.1 iPXE builds

### Requirement: Render the second-stage boot script with runtime variables
The system SHALL write `state/assets/boot.ipxe` that uses iPXE's runtime `${next-server}` and `${mac:hexhyp}` variables. The script SHALL NOT hardcode any IPv4 address.

#### Scenario: boot.ipxe is IP-portable
- **WHEN** `nostos build` completes
- **THEN** `state/assets/boot.ipxe` references `${next-server}` and does not contain any literal IPv4 address

#### Scenario: Kernel URL includes /assets/ prefix
- **WHEN** the operator inspects `state/assets/boot.ipxe`
- **THEN** the `kernel` directive references `/assets/vmlinuz-<arch>` (not `/vmlinuz-<arch>`), matching the serve layout where the HTTP root is the state directory

### Requirement: Serve HTTP + DHCP + TFTP for PXE booting with correct root
The system SHALL start an HTTP server on the configured port (default 9080) with document root `<state-dir>/` AND a `dnsmasq` process providing DHCP + TFTP, filtered to PXE clients only.

#### Scenario: Both assets and configs are reachable
- **WHEN** `nostos serve` is running and a client sends `GET /assets/boot.ipxe` and `GET /configs/<mac>.yaml`
- **THEN** both return HTTP 200 with the respective file contents

#### Scenario: Serve does not interfere with non-PXE DHCP
- **WHEN** `nostos serve` is running and a non-PXE device on the LAN sends DHCPDISCOVER
- **THEN** nostos's dnsmasq does not respond, leaving the consumer's existing DHCP server authoritative

#### Scenario: Stale HTTP processes cleaned on start
- **WHEN** `nostos serve` runs while a previous HTTP server still holds the port
- **THEN** the stale process is terminated and the new server binds successfully

### Requirement: One-shot wipe flag applied to kernel cmdline
The system SHALL inspect `state/pending-wipes.json` when rendering `boot.ipxe` during a serve session. For every MAC present in that file, the kernel cmdline SHALL include `talos.experimental.wipe=system`.

#### Scenario: Wipe flag is rendered when queued
- **WHEN** `nostos wipe dell01` adds `d0:94:66:d9:eb:a5` to pending-wipes AND `nostos serve` starts
- **THEN** `state/assets/boot.ipxe` contains `talos.experimental.wipe=system` on the kernel line

#### Scenario: Wipe flag auto-clears after successful install
- **WHEN** `nostos up dell01` completes successfully (node comes back Ready)
- **THEN** the entry for `d0:94:66:d9:eb:a5` is removed from pending-wipes AND a subsequent `nostos serve` renders boot.ipxe without the wipe flag

#### Scenario: Boot.ipxe is restored on nostos up error
- **WHEN** `nostos up dell01` fails partway through (error, SIGINT)
- **THEN** the rendered boot.ipxe is reverted to its pre-up state, preventing an unintended wipe on a later unrelated PXE boot
