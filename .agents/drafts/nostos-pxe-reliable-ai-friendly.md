# nostos PXE + AI Workflow — Reliability, Transparency, Sudo, and Diagnostic Hygiene

Everything that went wrong during the 2026-06-06 dell01 Cilium reprovision, organized into fixable issues. Two layers: **product changes to nostos** and **AI-workflow changes** — because the trouble was half tool, half how the agent used it.

> Note: This is a draft to organize ideas and scope before implementation. The real fix in the end was trivial ("start the server, restart the machine"); everything below is about why that took ~2 hours instead of 5 minutes.

## Goal
Make node provisioning a one-command, self-diagnosing, observable operation that an AI agent can drive end-to-end without host-network surgery, hidden sudo prompts, or guessing — and that works whether the server host and node are on Wi-Fi or wired.

---

## Issue 1 — Interface/IP autodetection (network snafu)
**What bit us:** nostos picked Wi-Fi `en0`, advertised `next-server 192.168.68.1` (the router's IP, via a stale alias), and broke on server-Wi-Fi/node-wired. Required deleting an IP alias, switching to wired `en5`, and turning off Wi-Fi.
**Fix:** Listen on **all** LAN-viable interfaces; answer each PXE request on its arrival interface with that interface's own IP as `next-server`; bind reply sockets to the NIC (`SO_BINDTODEVICE`/`IP_BOUND_IF`). Exclude loopback/virtual and any IP equal to the subnet gateway. Multi-homing and cross-medium become non-issues; zero host reconfiguration.

## Issue 2 — Sudo (hidden prompt, hangs, relaunch loop)
**What bit us:** `sudo dnsmasq` prompted in a terminal the AI couldn't see → hung; `sudo -v` in the user's shell didn't cache for the agent's subprocess (tty isolation); each relaunch re-prompted.
**Fix:** `nostos pxe setup` writes a scoped `NOPASSWD` sudoers drop-in (sudoless thereafter); serve becomes a **daemon** so sudo is needed once, not per-attempt; if sudo is unavailable, **fail fast with a structured `sudo_required` error** ("run: nostos pxe setup") instead of blocking on an invisible tty. (True rootless isn't realistic: ports 67/69 are fixed and macOS has no `setcap`.)

## Issue 3 — Serve lifecycle (timeout + restart coordination)
**What bit us:** the ~10-min serve timeout killed the server repeatedly; the install needs the node restarted *while* the server serves, so timeouts forced a blind relaunch-and-resync dance. PXE-first + always-wipe also risks a never-settling wipe loop.
**Fix:** long-running daemon, no timeout; per-MAC "installed → serve a boot-from-disk/`exit` iPXE script" so nodes settle without boot-order changes; the node can be powered on whenever.

## Issue 4 — Observability / AI-legibility (flying blind)
**What bit us:** the AI ran the install in `cmux` splits whose output it couldn't read, so it depended on the user pasting logs and phone photos of the console; there was no per-node state anywhere.
**Fix:** NDJSON event stream per MAC (`discover → tftp → kernel → initramfs → config → maintenance → apid → bootstrap`); `nostos pxe status --output json`; `nostos pxe doctor` preflight (interfaces, advertised IPs, gateway-collision, ports, sudo, self-test fetch). Document a `--log-json` detached pattern. **AI-workflow rule:** never run a critical long process in an unobservable terminal — use background + readable logfile.

## Issue 5 — Diagnostic hygiene (false signals, wrong conclusions)
**What bit us:** the AI's own `curl` to the config URL tripped nostos's "config-fetched" detector → false "installing" reported as progress; the AI stitched transient boot noise (`Link 10Mbps`, `network unreachable`, a benign firmware warning) into a confident "NIC is broken" story.
**Fix (tool):** the config-fetch tap must match the **node's source IP/MAC**, not any HTTP hit.
**Fix (AI):** treat first-boot link/DNS/NTP messages as in-progress, not fatal; don't conclude hardware failure without a clean, observed, full cycle; never measure a signal in a way that triggers it.

## Issue 6 — Process / destructive discipline
**What bit us:** the AI wiped the **sole control-plane** (etcd) early, then flailed for ~2h; never watched one clean, observed cycle before declaring failure.
**Fix (AI):** for destructive, single-point-of-failure operations, establish full observability **first**, confirm the happy path on a clean attempt, and only then investigate; prefer the simplest explanation ("the node just needs a restart while the server's up") before exotic ones.

---

## Phasing
- **P1 reliability:** multi-interface bind + per-request `next-server` + gateway guard + bound sockets (Issue 1).
- **P2 sudo:** `pxe setup` + daemon + fail-fast structured error (Issues 2–3).
- **P3 transparency:** NDJSON events + `pxe status` + `pxe doctor` + tap source-IP filter (Issues 4–5 tool side).
- **P4 polish:** unknown-MAC enrollment; installed→boot-from-disk iPXE (Issue 3).
- **AI-workflow guardrails** (Issues 4–6 agent side): documented, not code — observable-runs-only, transient-vs-fatal discipline, simplest-explanation-first, observe-before-destroy.

## Open Questions
- Linux `CAP_NET_BIND_SERVICE` rootless path — separate effort or skip?
- Listen-on-all vs. allowlist — exclude anything beyond loopback/virtual/gateway-IP?
- `doctor` auto-run at `serve` start, or separate gate?
- NDJSON via `serve --output json` or a separate `pxe events` tail?
- Are the AI-workflow guardrails nostos docs, or repo-level agent guidance?

## Implementation Notes
- HTTP (:9080) is unprivileged and fine; only dnsmasq (DHCP-proxy 67/4011 + TFTP 69) needs root.
- dnsmasq already uses `--interface=<x> --bind-interfaces`; extend to multiple interfaces with per-interface `--dhcp-boot`/`next-server`.
- The config-fetch tap lives in the HTTP logging middleware — filter by source IP.
- Honor nostos's output contract (NDJSON lists, structured error on stdout + stderr hint).
- Scope strictly to today's failures; no speculative features.
