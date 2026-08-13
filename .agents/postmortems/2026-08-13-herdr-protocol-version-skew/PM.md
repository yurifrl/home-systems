---
date: 2026-08-13
status: draft
incident_status: resolved
sessions:
  - 019ff856-5124-7130-8052-4cb64cccfee8
components:
  - herdr
  - herdr-phone
  - cloudflared
  - workstation
symptoms:
  - herdr.syscd.space unreachable (Access login loops, never reaches the app)
  - "herdr handshake failed (is Herdr running?): incompatible: herdr protocol 19 is not supported; require 17"
  - "error: herdr server is already running"
  - nothing listening on 127.0.0.1:8787
failure_mode: herdr-self-update-protocol-skew
affected_urls:
  - https://herdr.syscd.space
beads: []
memories: []
supersedes: []
related: []
---

# Postmortem: Herdr self-update broke the relay's wire protocol

- **Severity/Impact:** `herdr.syscd.space` served no application for roughly 4.5 hours of confirmed downtime (self-updated Herdr binary started Aug 12 21:34, mitigated Aug 13 02:20 UTC). Remote supervision of the workstation's coding agents was unavailable from outside the LAN. Single user affected (Yuri); no data loss.
- **Root cause (one line):** `herdr-self-update-protocol-skew` — an out-of-band self-update to Herdr 0.8.0 moved the socket API to protocol 19 while `herdr-phone` was pinned to 0.7.5/protocol 17, so the relay refused the handshake and never bound its origin port.

## What Happened

Two components speak a versioned protocol over a Unix socket: `herdr server` (the substrate) and `herdr-phone serve` (the relay that Cloudflare Tunnel dials on `127.0.0.1:8787`). NixOS pinned both — Herdr 0.7.5 and the `herdr-phone` fork at rev `382f918` — so they agreed on protocol 17.

Herdr, however, self-updates. A manually self-updated `herdr 0.8.0` was installed outside Nix at `/home/yuri-workstation/.local/bin/herdr` and started on Aug 12 21:34, and it won the race for the socket `~/.config/herdr/herdr.sock`. That produced two coupled failures. First, the Nix-managed `herdr-server` (0.7.5) could no longer bind the socket and crash-looped with `error: herdr server is already running` — 182 restarts by the time it was diagnosed. Second, and the actual outage: 0.8.0 answers the handshake with `"protocol": 19`, but `herdr-phone` v0.4.0 required exactly 17, so it exited with `incompatible: herdr protocol 19 is not supported; require 17` before ever binding `:8787`. With no origin listening, the tunnel had nothing to serve.

The systemd wiring amplified it: `herdr-phone` declares `Requires=herdr-server.service`, so the crash-looping server restarted the relay every ~3 seconds even after the relay itself was capable of running.

The fix moved the whole stack forward to 0.8.0/protocol 19 rather than downgrading, because the box self-updates and a downgrade would simply re-break. Before changing the constant, every RPC method (23), event name (27), and JSON field (95) the relay decodes was verified present in the real 0.8.0 binary, and the socket decoder was confirmed to tolerate unknown fields (`DisallowUnknownFields` applies only to inbound browser/pairing paths) — so the protocol constant was the sole incompatibility, with no wire-format rework needed.

## Detection Gap (how we catch it next time)

- **What the user saw first:** `herdr.syscd.space` was unusable from outside — the Cloudflare Access login never landed on a working app. No alert fired; the user reported it.
- **How we detect it before the user next time:** Gatus has **no herdr endpoint at all** (`modules/gatus/config.yaml` covers argocd, rpg, obs, alertmanager, ha, prometheus, zigbee2mqtt, teleport, plus tailnet/apiserver pings — the workstation box is entirely uncovered). Critically, a naive status probe would **not** have caught this: throughout the outage the public URL still returned `302` to the Access login, because Cloudflare Access answers before the origin is ever consulted. The detecting signal must be an *authenticated* check that reaches the relay and asserts on the body/status behind Access, not a bare status check on the public URL.
- **Fix path once detected:** confirm the socket owner and the protocol both sides speak, then align the pin and restart — full commands in Mitigation below.

## Mitigation (runbook — how to detect & fix this again)

**Symptom triage.** From outside, a `302` to `*.cloudflareaccess.com` proves only that the edge is alive — it says nothing about the origin. Check the origin from the box:

```bash
ssh root@100.103.90.127   # workstation-company; 'workstation' is on the personal tailnet
systemctl is-active herdr-server herdr-phone
ss -ltnp | grep 8787                       # no listener => relay never started
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8787/   # want 200
```

Note the tunnel unit is **not** `cloudflared.service` — NixOS names it per tunnel id
(`cloudflared-tunnel-<uuid>.service`), so `systemctl is-active cloudflared` reports a
misleading `inactive`. Find it with:

```bash
systemctl list-units --all | grep -i cloudflar
```

**Diagnose protocol skew.** The relay names the mismatch verbatim:

```bash
journalctl -u herdr-phone -n 30 --no-pager | grep -i handshake
# incompatible: herdr protocol 19 is not supported; require 17
```

Find who actually owns the socket, and which binary it is:

```bash
ss -lxp | grep herdr.sock                  # -> pid of the listening 'herdr'
ls -l /proc/<pid>/exe                      # /nix/store/...-herdr-X.Y.Z vs /home/*/.local/bin/herdr
```

A `/home/*/.local/bin/herdr` owner means a self-update escaped Nix. Read the protocol number each binary reports (it is in the ping/handshake payload) before choosing a direction:

```bash
grep -ao '"protocol"[: ]*[0-9]*' /nix/store/*-herdr-*/bin/herdr | sort -u
```

**Fix — move the pin forward (preferred).** Downgrading loses to the next self-update.

1. In the fork, set `Protocol` / `MinHerdrVersion` (`internal/herdr/doc.go`, `internal/buildinfo/buildinfo.go`, `herdr-plugin.toml`), then `go build ./... && go vet ./... && go test ./...` and the web suite. Push and record the rev.
2. In `.submodules/nixos/modules/workstation/herdr-packages.nix`, bump the `herdr` `version`/`url`/`hash` and the `herdr-phone` `rev`/`sha256`. `vendorHash`/`npmDepsHash` only change if `go.mod`/`go.sum`/`package-lock.json` changed. Get hashes on the box (no Nix on the Mac):
   ```bash
   ssh root@100.103.90.127 'nix store prefetch-file --json <release-url>'
   ssh root@100.103.90.127 'nix-prefetch-url --unpack --type sha256 https://github.com/yurifrl/herdr-phone/archive/<rev>.tar.gz'
   ```
3. Commit/push `nixos`, then `ssh root@100.103.90.127 ws-rebuild` (it pulls `/root/nixos` and switches). Expect ~10–15 min: it compiles the Go relay and the Vite PWA.

**Hand the socket back to Nix.** Until the out-of-band binary stops, `herdr-server` cannot bind. This kills the manual server's live sessions:

```bash
systemctl stop herdr-server            # stop the restart loop first
kill -TERM <manual-pid>                # /home/*/.local/bin/herdr server
systemctl start herdr-server && systemctl restart herdr-phone
```

**Verify.** Both units `active` with `NRestarts=0`, the socket owned by a `/nix/store/...` pid, `origin=200`, and the public URL `302` to Access:

```bash
systemctl show herdr-server herdr-phone -p ActiveState -p NRestarts
ss -lxp | grep herdr.sock
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8787/
curl -sS -o /dev/null -w '%{http_code}\n' https://herdr.syscd.space/
```

**Gotchas.** `/healthz` on the relay is not a route — the SPA fallback returns `200` for any unknown path, so it is worthless as a health assertion. Long `ssh` commands from this Mac get killed by the tool timeout; detach long work and poll in short commands (killing the ssh session does **not** kill an already-running `nixos-rebuild` — it keeps going as an orphan). Node 26 on the Mac has no `localStorage`, so the web suite needs `NODE_OPTIONS=--localstorage-file=<path>`.

## Dead Ends

- **`systemctl is-active cloudflared` returning `inactive`** looked like a second, independent failure (dead tunnel). It was a phantom: the unit is `cloudflared-tunnel-c8196d6d-….service`, and the real tunnel was active and healthy the whole time. Wasted a diagnostic cycle on the wrong component.
- **`ws-rebuild` printing "Command aborted"** read like a failed build. The build was fine — the tool timeout killed my *ssh session*, while `nixos-rebuild` kept running as an orphan (PID 398405). Starting a second rebuild would have fought the first for the Nix lock; checking `ps` for the existing pid was the right move.
- **22 failing web tests** looked like fallout from the protocol change. They were pre-existing environment breakage — Node 26 refuses `localStorage` without `--localstorage-file`, and two of the failing files (`prefs.test.ts`, `relay-mode.test.ts`) were provably untouched (`git status --porcelain` clean). All 379 passed once the flag was supplied.
- **The public `302`** invited the conclusion that the edge was broken. It was correct behaviour from a healthy edge and actively masked the dead origin — the same property that makes a naive status check useless here.
- **`herdr-server`'s 182 restarts** were the loudest signal and drew attention first, but they were a *symptom* of the socket squatter, not the outage cause. The relay's handshake rejection was the outage; the server crash-loop only added the 3-second relay flapping.

## Timeline

### 2026-08-12
- `21:34` A self-updated `herdr 0.8.0` is installed at `/home/yuri-workstation/.local/bin/herdr`, outside Nix, and starts as `herdr server` (PID 248361). It takes ownership of `~/.config/herdr/herdr.sock` and speaks protocol 19.
- `21:34` Nix-managed `herdr-server` (0.7.5) can no longer bind the socket and begins crash-looping: `error: herdr server is already running`.
- `21:34` `herdr-phone` v0.4.0 (protocol 17) rejects the 0.8.0 handshake and exits before binding `:8787`. `herdr.syscd.space` stops serving the app. No alert fires.

### 2026-08-13
- `~01:20` User reports `herdr.syscd.space` down.
- `01:25` Edge verified healthy: DNS resolves to Cloudflare, public URL returns `302` to the Access login with the expected AUD. Attention moves to the origin.
- `01:30` On the box: nothing listening on `:8787`; `herdr-phone` journal shows `incompatible: herdr protocol 19 is not supported; require 17`. Root cause identified.
- `01:35` Protocol numbers confirmed directly from the binaries: Nix 0.7.5 → 17, `~/.local/bin` 0.8.0 → 19.
- `01:40` Decision: move the stack forward to 0.8.0/protocol 19 rather than downgrade, because the box self-updates and a downgrade would re-break.
- `01:45` Compatibility audit against the real 0.8.0 binary: all 23 RPC methods, 27 events, and 95 decoded JSON fields present; socket decoder tolerates unknown fields. Protocol constant is the only incompatibility.
- `01:48` Failing test written first (`internal/herdr/protocol19_test.go`), reproducing the production rejection; then the constants moved to 19 / `0.8.0`. Go suite green.
- `01:49` `ws-rebuild` started on the workstation; the ssh session is killed by the tool timeout and prints `Command aborted`.
- `01:52` Nix pins pushed (`nixos` `c5bf314`): herdr `0.8.0`, `herdr-phone` rev `a662074`.
- `01:57` Orphaned `nixos-rebuild` (PID 398405) found still building — compiling the Go relay and the Vite PWA. Left alone rather than restarted.
- `02:05` Switch completes. Both units now reference `herdr-0.8.0`; `:8787` is **listening** and the origin answers `200`. Outage over. `herdr-server` still crash-loops (counter 182) against the squatted socket, flapping the relay every ~3s.
- `02:10` `cloudflared` reported `inactive` — traced to the per-tunnel unit name; the real tunnel was active all along.
- `02:20` With user approval, the manual 0.8.0 server (PID 248361) is stopped and the socket handed to Nix. `herdr-server` + `herdr-phone` come up `active` with `NRestarts=0`, socket owned by `/nix/store/…-herdr-0.8.0`, origin `200`, public URL `302` to Access. Fully resolved.
