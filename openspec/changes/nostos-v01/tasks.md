## 1. Scaffold the package

- [x] 1.1 Create `.submodules/nostos/` directory with `pyproject.toml` (uv-managed, Python >=3.11, entry point `nostos = "nostos.cli:main"`)
- [x] 1.2 Create `.submodules/nostos/src/nostos/__init__.py` with `__version__ = "0.1.0"`
- [x] 1.3 Add dev dependencies to `pyproject.toml`: `pytest`, `ruff`, `mypy`
- [x] 1.4 Add runtime dependencies: `click`, `rich`, `questionary`, `httpx`, `pyyaml`, `pydantic`
- [x] 1.5 Create `.submodules/nostos/tests/` with `conftest.py` and `test_smoke.py` verifying the package imports
- [x] 1.6 Create `.submodules/nostos/README.md` with install + quickstart blurb

## 2. Configuration model (pydantic)

- [x] 2.1 Create `src/nostos/config.py` with pydantic models: `ClusterConfig`, `SecretsConfig`, `NodeConfig`, `NostosConfig`
- [x] 2.2 Implement MAC validation (six hex octets separated by colons) on `NodeConfig.mac`
- [x] 2.3 Implement `NostosConfig.load(path)` that reads YAML, validates, and errors with human-readable messages on schema violations
- [x] 2.4 Implement duplicate-MAC detection across nodes
- [x] 2.5 Write unit tests covering valid config, invalid MAC, duplicate MAC, missing required fields

## 3. Config discovery

- [x] 3.1 Create `src/nostos/discovery.py` with a `find_config()` function implementing the search order (--config flag → $NOSTOS_CONFIG → cwd + parents)
- [x] 3.2 Write unit tests for each discovery path

## 4. Secrets backend contract

- [x] 4.1 Create `src/nostos/secrets/__init__.py` exposing `SecretsBackend` Protocol with `resolve(uri) -> str` and `validate() -> None`
- [x] 4.2 Create `src/nostos/secrets/registry.py` mapping URI schemes → backend classes
- [x] 4.3 Implement `src/nostos/secrets/onepassword.py` using `op` CLI subprocess calls; validate via `op whoami`
- [x] 4.4 Implement `src/nostos/secrets/env.py` (reads environment variables)
- [x] 4.5 Implement `src/nostos/secrets/file.py` (reads filesystem paths)
- [x] 4.6 Implement `src/nostos/secrets/sops.py` (shells out to `sops --decrypt`)
- [x] 4.7 Write a single `resolve_template(template_text, backend) -> rendered_text` that finds all `<scheme>://...` URIs in YAML and replaces with resolved values, preserving YAML structure
- [x] 4.8 Write unit tests using a mock backend that never hits real secrets
- [x] 4.9 Verify secrets never appear in log output (regex scan of captured logs in tests)

## 5. Node registry

- [x] 5.1 Create `src/nostos/registry.py` exposing `list_nodes()`, `get_node(name)`, `add_node(...)`, `remove_node(name)`, `render_node(name)`
- [x] 5.2 Implement `render_node` that loads the template, runs `resolve_template` against the configured backend, writes to `state/configs/<mac-hyphenated>.yaml`
- [x] 5.3 Implement MAC-to-hyphen conversion (`d0:94:66:d9:eb:a5` → `d0-94-66-d9-eb-a5`)
- [x] 5.4 Add `talosctl validate --config <output> --mode metal` as post-render sanity check (skip if talosctl absent, warn)
- [x] 5.5 Implement reachability probe: ping + TCP:50000 check, return pydantic `NodeStatus`
- [x] 5.6 Implement Talos version probe using `talosctl version --nodes <ip>` + talosconfig
- [x] 5.7 Write unit tests with a fixture consumer repo

## 6. PXE asset build

- [x] 6.1 Create `src/nostos/pxe/build.py`
- [x] 6.2 Download Talos kernel + initramfs from `factory.talos.dev` using `httpx`, cache in `state/assets/`
- [x] 6.3 Detect and use `docker` to build iPXE `snponly.efi` with an embedded retry-loop script (`retry_dhcp` + `isset ${filename}`)
- [x] 6.4 Render `state/assets/boot.ipxe` using `${next-server}` (no hardcoded IP); parameterize Talos version
- [x] 6.5 Assert `ipxe.efi` size < 256 KB, fail loud if not
- [x] 6.6 Skip rebuild when inputs unchanged (checksum of schematic ID + version + embed script)
- [x] 6.7 Unit test for the `boot.ipxe` rendering; integration test (gated on Docker availability) for the full build

## 7. PXE serve

- [x] 7.1 Create `src/nostos/pxe/serve.py` with `start_http_server()` and `start_dnsmasq()` using subprocess
- [x] 7.2 Auto-detect the ethernet interface carrying the target subnet (replaces `detect-mac-ip.sh`)
- [x] 7.3 Build dnsmasq command-line with PXE vendor-class filtering so it co-exists with the consumer's router DHCP
- [x] 7.4 Stage `ipxe.efi` at `/tmp/nostos-tftp/` with mode 0644 (dnsmasq drops to nobody on macOS)
- [x] 7.5 Kill any stale HTTP server on the configured port before binding
- [x] 7.6 Implement clean shutdown: trap SIGINT/SIGTERM, kill children, remove staged TFTP files
- [x] 7.7 Write logs to `state/logs/serve-<timestamp>.log`
- [x] 7.8 Add `nostos serve --down` to kill any running nostos serve without relying on process state

## 8. Cluster control

- [x] 8.1 Create `src/nostos/cluster/bootstrap.py` wrapping `talosctl bootstrap`
- [x] 8.2 Implement wait-for-etcd-healthy loop with configurable timeout (default 5 min)
- [x] 8.3 Reject bootstrap on non-controlplane nodes by checking `NodeConfig.role`
- [x] 8.4 Make bootstrap idempotent (detect already-bootstrapped state and exit success)
- [x] 8.5 Fetch kubeconfig to `state/kubeconfig` after successful bootstrap
- [x] 8.6 Create `src/nostos/cluster/cert.py` implementing `refresh_admin_cert()`:
  - [x] 8.6.1 Read CA cert + key from a rendered machineconfig or directly from secrets backend
  - [x] 8.6.2 Generate Ed25519 keypair, CSR with `os:admin` role, sign with CA
  - [x] 8.6.3 Use Python `base64` module directly (do NOT shell out to macOS `base64`, which emits CRLF)
  - [x] 8.6.4 Write `state/talosconfig` with embedded new cert
  - [x] 8.6.5 Validate by calling `talosctl version --nodes <controlplane>` and expecting success
- [x] 8.7 Implement `src/nostos/cluster/status.py` reporting per-node reachability, apid state, version, kubelet health
- [x] 8.8 Implement `src/nostos/cluster/wipe.py` with one-shot flag registry persisted to `state/pending-wipes.json`; the serve layer reads this when rendering per-node `boot.ipxe` content

## 9. CLI surface (click commands)

- [x] 9.1 Create `src/nostos/cli.py` with top-level `@click.group()`
- [x] 9.2 Implement `init` command (scaffold config.yaml, templates/, .gitignore for state/)
- [x] 9.3 Implement `node add`, `node list`, `node remove` subgroup
- [x] 9.4 Implement `build` calling `pxe.build`
- [x] 9.5 Implement `render <node>` calling `registry.render_node`
- [x] 9.6 Implement `serve` calling `pxe.serve`
- [x] 9.7 Implement `install <node>` (printer-only: emit per-node BIOS + PXE cheat-sheet to stdout)
- [x] 9.8 Implement `wipe <node>` marking node in pending-wipes
- [x] 9.9 Implement `bootstrap <node>`
- [x] 9.10 Implement `config refresh` with `--hours` flag (default 876000)
- [x] 9.11 Implement `status`
- [x] 9.12 Implement `kubeconfig` (refresh only)
- [x] 9.13 Implement `nuke` (`rm -rf state/` after y/N confirmation)
- [x] 9.14 Implement `web` (depends on section 10)
- [x] 9.15 Add global flags: `--config`, `--output text|json`, `--debug`
- [x] 9.16 Implement error formatter that hides tracebacks unless `--debug`

## 10. Web dashboard

- [x] 10.1 Add FastAPI, uvicorn, jinja2 to dependencies
- [x] 10.2 Create `src/nostos/web/app.py` with FastAPI app bound to `127.0.0.1`
- [x] 10.3 Refuse non-loopback bind unless `--i-know-what-im-doing` passed
- [x] 10.4 Implement REST: `GET /api/nodes`, `GET /api/nodes/{name}`, `GET /api/status`
- [x] 10.5 Implement mutation endpoints: `POST /api/nodes/{name}/wipe`, `POST /api/nodes/{name}/bootstrap`, `POST /api/nodes/{name}/refresh`
- [x] 10.6 Every mutation endpoint validates a `confirmation: <node-name>` body field
- [x] 10.7 Implement `--read-only` flag that short-circuits mutation endpoints to HTTP 403
- [x] 10.8 Create `src/nostos/web/static/` + `src/nostos/web/templates/` with a single-page dashboard (vanilla JS, no build step)
- [x] 10.9 UI: node table with status pills, auto-refresh every 10 s
- [x] 10.10 UI: "Copy install steps" button per node that copies a formatted cheat-sheet to clipboard
- [x] 10.11 UI: mutation buttons behind typed-name confirmation modal

## 11. End-to-end verification

- [x] 11.1 Write an integration test that runs `nostos init` in a tmp dir, adds a node, renders (using a mock backend), and asserts the rendered file matches golden output
- [~] 11.2 Verify parity against the current `pxe/scripts/*.sh` output for the dell01 case (byte-diff the rendered config) — deferred to adopt-nostos (needs real op CLI + fixtures from home-systems repo)
- [~] 11.3 Run `nostos build` in CI (Docker available) and assert `ipxe.efi` is under 256 KB — gated integration test in place, requires Docker at runtime
- [~] 11.4 Manual checklist: run `nostos serve` locally, power on Dell, confirm full boot sequence matches expectations — runbook in README; operator-driven, not CI-automatable

## 12. Packaging and docs

- [x] 12.1 Write `.submodules/nostos/README.md` with install, quickstart, config schema reference
- [x] 12.2 Add `.submodules/nostos/CHANGELOG.md` with v0.1.0 entry
- [x] 12.3 Add `.submodules/nostos/LICENSE` (MIT)
- [x] 12.4 Ensure `uv tool install --editable .submodules/nostos` works from the home-systems repo root
- [~] 12.5 Tag v0.1.0 within the `.submodules/nostos/` tree once all preceding tasks complete — left to operator; tool is in-tree, not yet a standalone git repo
