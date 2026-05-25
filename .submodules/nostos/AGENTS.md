# AGENTS.md — nostos invariants for AI agents

This file documents non-obvious invariants and operational guarantees for
agents (and humans) driving `nostos`. Every rule here either has a
corresponding test in `internal/cli/...` or a schema entry surfaced via
`nostos schema`.

## Output contract

Every command supports `--output text|json`. Default is `text` (TTY-friendly).
Pass `--output json` to get parseable output:

- **List commands** emit **NDJSON**: one JSON object per line, no array
  wrapper. Examples: `nostos node list`, `nostos secrets list`,
  `nostos secrets keys list`, `nostos schema` (no args).
- **Object commands** emit a single pretty-printed JSON object. Examples:
  `nostos node show`, `nostos status`, `nostos schema <method>`.
- **Mutating commands with `--dry-run`** emit a **Plan** envelope:

  ```json
  {"status": "preview", "method": "node.install", "would_execute": [{"phase":"...","detail":"..."}]}
  ```

Errors emit a JSON object on **stdout**, with a one-line hint on **stderr**.

## Exit codes

```
 0 success
10 validation_failed   (bad input, schema violation, parse error)
11 network_error       (timeout, refused, DNS)
12 auth_error          (op session, BMC creds, OAuth scope)
13 conflict            (lock held, node already ready, dup MAC)
14 not_found           (node not in config, key id absent)
15 timeout             (operation deadline exceeded)
 1 internal_error      (everything else; treat as bug-in-nostos)
```

Get the live catalog:

```bash
nostos schema --exit-codes
```

## Required ordering

1. **Before any `node install`**: `nostos secrets test tailscale` to confirm
   the OAuth client is valid and Tailscale will accept the auth-key mint.
2. **After editing `secrets:` in config.yaml**: re-run
   `nostos secrets test tailscale`. A stale session will fail at install time
   *after* a node is wiped, leaving the node in PXE-loop limbo.
3. **Before any cluster mutation** (install, cleanup, render to a live node):
   verify the cluster is healthy:
   ```bash
   nostos status --output json | jq -e '.cluster.healthy'
   ```
4. **After any structural change**: run `nostos render <node> --dry-run`
   first to confirm the would-be output before writing.

## Always pass `--reinstall` when re-flashing

`nostos node install <name>` short-circuits if the node is already Ready
(method=tpi guards this; method=pxe relies on bootcmd state). Pass
`--reinstall` to force the flow:

```bash
nostos node install tp1 --reinstall --yes
```

Without `--reinstall` you may get a misleading `conflict` (exit 13) when the
flock is held or the node is reporting Ready.

## Idempotency guarantees

| Command                          | Idempotent | Destructive | Confirm |
|----------------------------------|-----------|-------------|---------|
| `init`                           | yes       | no          | no      |
| `build`                          | yes       | no          | no      |
| `render`                         | yes       | no          | no      |
| `node list` / `node show`        | yes       | no          | no      |
| `status`                         | yes       | no          | no      |
| `secrets list` / `secrets test`  | yes       | no          | no      |
| `secrets keys list`              | yes       | no          | no      |
| `schema`                         | yes       | no          | no      |
| `node install`                   | no        | YES         | `--yes` |
| `node remove`                    | no        | YES         | `--yes` |
| `nuke`                           | no        | YES         | `--yes` |
| `wipe`                           | no        | YES         | no      |
| `bootstrap`                      | no        | YES         | no      |
| `secrets keys revoke`            | yes (404 OK) | YES      | no      |
| `cluster cleanup`                | depends   | YES         | `--yes` + `--really-yes` |

Safe to retry under any failure: `init`, `build`, `render`, `status`,
`node list`, `node show`, `secrets list`, `secrets test`, `secrets keys
list`, `schema`. All read-only or tolerant of repeated writes.

## `--dry-run` semantics

When supported (`node install`, `render`, `secrets keys revoke`,
`cluster cleanup`), `--dry-run`:

- Spawns **zero** subprocesses.
- Emits a `{"status":"preview","would_execute":[...]}` envelope.
- Exits **0** (the preview is the success).
- The emitted `would_execute[]` is a (super)sequence of what would actually
  run without `--dry-run`. Use it to plan the next step.

## Rate limits

Documented for clients orchestrating bulk operations:

- **BMC probe / preflight** (Turing Pi): max **1 probe per host per 1s**;
  exponential backoff to 5s on failure. Don't fan out probes.
- **Tailscale OAuth API**: 60 req/min per client. `secrets test tailscale`
  mints + revokes one key per call (2 requests). Keep test cadence below
  1/sec to leave headroom for `mint-key` during real installs.
- **`talosctl version`** (used in `status`): 1.5s wall-clock per node;
  parallelism is not throttled, so `nostos status` against a 100-node
  cluster will fan out 100 simultaneous probes. For larger fleets, prefer
  `--fields` projection and serial polling.

## Never pass user-provided strings as `--json` without escaping

When invoking `nostos` from another tool, sanitize:

- Node names against `^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$` (the validator does
  this server-side; pre-sanitize so a UI can show a hint earlier).
- 1Password references against `^op://[\w-]+/[\w.-]+(/[\w.-]+){0,2}$`.
- Field masks against the schema returned by `nostos schema <method>`.

The CLI strips ASCII control chars and ANSI CSI sequences before echoing
user input into JSON output, but you should still avoid passing them.

## MCP integration

`nostos mcp` starts a JSON-RPC 2.0 MCP server on stdio. Single source of
truth: every tool name is `nostos.<method-id>` where `<method-id>` matches
the dot-path returned by `nostos schema`. Tool input schemas are derived
from cobra flags + the schema registry, so adding a flag automatically
propagates to MCP.

```bash
echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | nostos mcp | jq '.result.tools[].name'
```

A `tools/call` invocation returns the same JSON payload as
`nostos <command> --output json`.

## Where to find more

- `nostos schema` — every method, flag, exit code.
- `nostos schema --exit-codes` — the exit-code catalog as JSON.
- `nostos schema <method>` — full descriptor for one method.
- `internal/cli/errs/` — typed error contract.
- `internal/cli/inputx/` — input validators (run under `go test -fuzz=...`).
- `internal/mcp/` — JSON-RPC server.
