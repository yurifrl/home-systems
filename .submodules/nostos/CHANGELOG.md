# Changelog

All notable changes to `nostos` are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.0] — 2026-05-02

Initial release, developed in-tree at `.submodules/nostos/` inside
`yurifrl/home-systems`. Will be extracted to its own repo at v0.2.

### Added
- `nostos` CLI built on click, entry point `nostos = nostos.cli:main`.
- Top-level commands: `init`, `node add|list|remove`, `build`, `render`,
  `serve`, `install`, `wipe`, `bootstrap`, `config refresh`, `status`,
  `kubeconfig`, `nuke`, `web`.
- Global flags: `--config`, `--output text|json`, `--debug`.
- Pluggable secrets backend with adapters for `op://` (1Password),
  `sops://`, `env://`, `file://`. Selected via `secrets.backend` in
  `config.yaml`.
- Pydantic-validated `config.yaml` schema: cluster meta + secrets +
  nodes. MAC validation, node-name conventions, duplicate-MAC detection.
- PXE asset pipeline: Talos factory kernel/initramfs download, iPXE
  cross-compile via Docker (`snponly.efi` < 256 KB), boot.ipxe rendered
  with `${next-server}` runtime variable.
- Embedded iPXE retry-loop that tolerates consumer-router DHCP races
  (`isset ${filename}` gate).
- One-shot disk-wipe registry backing `nostos wipe <node>` to safely
  trigger `talos.experimental.wipe=system` without infinite loops.
- PXE serving: HTTP (Python) + dnsmasq (subprocess) on the host's
  ethernet interface, filtered to PXE clients so it coexists with the
  consumer's router DHCP.
- Cluster control: `talosctl bootstrap`, kubeconfig fetch, admin-cert
  regeneration offline against the existing CA (solves the expired
  client-cert trap). 100-year default validity, configurable.
- Optional localhost web dashboard (FastAPI + vanilla JS) with node
  status auto-refresh, mutation endpoints behind typed-name confirmation,
  `--read-only` flag, `--i-know-what-im-doing` gate for non-loopback binds.
- Test suite with 70+ tests covering config, discovery, secrets, registry,
  PXE build, PXE serve (pure-logic), cluster operations, CLI, web, and
  an end-to-end integration test.

### Known limitations
- Docker required for iPXE build in v0.1. Pre-built binaries arrive in v0.2.
- `nostos bootstrap` and `nostos status --live` require a real cluster —
  integration tests stubbed.
- sops backend is a functional stub; operator must shell `sops` themselves
  for any complex extraction today.

[0.1.0]: https://github.com/yurifrl/nostos/releases/tag/v0.1.0
