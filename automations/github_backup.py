#!/usr/bin/env -S uv run --quiet --script
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Mirror-backup GitHub repos to a local directory.

Standalone by design: depends only on the Python standard library and the
`git` binary, so the exact same file runs on a laptop (`uv run
github_backup.py`) and inside an Argo Workflow container (`python -u
github_backup.py`). It reads nothing from the Argo environment.

Behavior per run:
  1. List repos from the configured orgs (all, private included with the PAT)
     and users (own repos via /user/repos, others via /users/<user>/repos).
  2. In parallel, `git clone --mirror` new repos / `git fetch --all --prune`
     existing ones under <state>/mirrors/<owner>/<repo>.git.
  3. Snapshot each mirror to <state>/snapshots/<owner>/<repo>/<ts>.tar.gz,
     keeping only the newest N, skipping repos whose refs haven't changed.
  4. Remove mirrors whose repos disappeared from GitHub (only when the repo
     listing was complete, so a partial API failure can't delete mirrors).
  5. Write <state>/manifest.json and exit non-zero if any repo failed.

Configuration (env, all optional except GHA_PAT):
  GHA_PAT            GitHub PAT. Needs read access to the target repos:
                     fine-grained -> "Contents: read-only", or classic -> repo scope.
  GHA_BACKUP_ORGS    Comma-separated org logins (default "NSXBet").
  GHA_BACKUP_USERS   Comma-separated user logins (default "yurifrl"; the PAT
                     owner's private repos are included via /user/repos).
  GHA_BACKUP_STATE   Root directory for mirrors/snapshots (default "/state").
  GHA_BACKUP_JOBS    Parallel sync workers (default 4).
  GHA_BACKUP_SNAPSHOTS  Snapshots to keep per repo (default 5; 0 disables).
  GHA_BACKUP_TIMEOUT Per-repo git/API timeout in seconds (default 1800).

Usage:
  uv run github_backup.py [--orgs NSXBet] [--users yurifrl] [--state /state]
                          [--jobs 4] [--snapshots 5] [--timeout 1800]
"""

from __future__ import annotations

import argparse
import hashlib
import json
import logging
import os
import shutil
import subprocess
import sys
import tarfile
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from urllib import error as urlerror
from urllib import request as urlrequest

API_BASE = os.environ.get("GITHUB_API", "https://api.github.com")
API_TIMEOUT = 30  # seconds per HTTP call
PER_PAGE = 100
LOG = logging.getLogger("github-backup")


@dataclass
class Repo:
    owner: str
    name: str
    clone_url: str
    is_fork: bool

    @property
    def key(self) -> str:
        return f"{self.owner}/{self.name}"

    @property
    def mirror_path(self) -> Path:
        return Path(env("GHA_BACKUP_STATE")) / "mirrors" / self.owner / f"{self.name}.git"

    @property
    def snapshot_dir(self) -> Path:
        return Path(env("GHA_BACKUP_STATE")) / "snapshots" / self.owner / self.name


@dataclass
class RepoResult:
    repo: str
    status: str = "ok"  # ok | failed
    detail: str = ""
    refs: int = 0
    size_bytes: int = 0
    seconds: float = 0.0
    snapshot: str = ""


@dataclass
class Listing:
    repos: list[Repo] = field(default_factory=list)
    complete: bool = True  # False if any page failed -> prune is skipped


def env(name: str, default: str | None = None) -> str:
    value = os.environ.get(name)
    if value is None or value == "":
        if default is None:
            raise SystemExit(f"missing required env {name}")
        return default
    return value


def now_ts() -> str:
    return datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")


def api(path: str, token: str, params: dict[str, str] | None = None) -> list | dict:
    """GET one JSON page from the GitHub API."""
    query = ""
    if params:
        from urllib.parse import urlencode

        query = "?" + urlencode(params)
    req = urlrequest.Request(
        f"{API_BASE}{path}{query}",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "github-backup-script",
        },
    )
    try:
        with urlrequest.urlopen(req, timeout=API_TIMEOUT) as resp:
            return json.load(resp)
    except urlerror.HTTPError as exc:
        if exc.code == 403 and exc.headers.get("X-RateLimit-Remaining") == "0":
            raise SystemExit("GitHub API rate limit exhausted; aborting run") from exc
        raise


def list_page(path: str, token: str, params: dict[str, str], listing: Listing) -> list:
    """Paginate a repos endpoint; mark the run incomplete on page failure."""
    out: list = []
    page = 1
    while True:
        try:
            chunk = api(path, token, {**params, "page": str(page), "per_page": str(PER_PAGE)})
        except Exception as exc:  # noqa: BLE001 - any listing failure must not prune
            listing.complete = False
            LOG.warning("listing %s page %d failed: %s", path, page, exc)
            break
        if not isinstance(chunk, list):
            listing.complete = False
            LOG.warning("listing %s page %d returned unexpected payload", path, page)
            break
        out.extend(chunk)
        if len(chunk) < PER_PAGE:
            break
        page += 1
    return out


def list_repos(orgs: list[str], users: list[str], token: str) -> Listing:
    """Enumerate target repos: org repos (private included with the PAT)."""
    listing = Listing()
    me = api("/user", token)["login"]
    for org in orgs:
        for raw in list_page(f"/orgs/{org}/repos", token, {"type": "all"}, listing):
            listing.repos.append(
                Repo(raw["owner"]["login"], raw["name"], raw["clone_url"], raw["fork"])
            )
    for user in users:
        path = "/user/repos" if user == me else f"/users/{user}/repos"
        params = {"affiliation": "owner"} if user == me else {}
        for raw in list_page(path, token, params, listing):
            listing.repos.append(
                Repo(raw["owner"]["login"], raw["name"], raw["clone_url"], raw["fork"])
            )
    # De-dupe (a repo can be reachable via both org and user listing).
    unique: dict[str, Repo] = {}
    for repo in listing.repos:
        unique[repo.key] = repo
    listing.repos = sorted(unique.values(), key=lambda r: r.key)
    return listing


def git(
    args: list[str],
    cwd: Path | None = None,
    timeout: int = 1800,
    env: dict[str, str] | None = None,
) -> str:
    """Run one git command; raise with a clean message on failure.

    `env` carries the credential helper + GHA_PAT, so the token is only in the
    subprocess environment, never in URLs (which git prints on errors).
    """
    proc = subprocess.run(
        ["git", *args],
        cwd=cwd,
        env=env,
        capture_output=True,
        text=True,
        timeout=timeout,
    )
    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout).strip().splitlines()
        raise RuntimeError(f"git {' '.join(args[:3])} failed: {detail[-1] if detail else 'unknown error'}")
    return proc.stdout


def refs_fingerprint(mirror: Path) -> str:
    """Stable hash over every ref + sha: detects new, moved, and deleted refs."""
    out = git(["for-each-ref", "--format=%(refname) %(objectname)"], cwd=mirror)
    return hashlib.sha256(out.encode()).hexdigest()


def snapshot(mirror: Path, dest_dir: Path, keep: int) -> str:
    """Tar the mirror; return the snapshot filename (empty if skipped)."""
    dest_dir.mkdir(parents=True, exist_ok=True)
    name = f"{now_ts()}.tar.gz"
    tmp = dest_dir / f".{name}.tmp"
    with tarfile.open(tmp, "w:gz") as tar:
        tar.add(mirror, arcname=mirror.name)
    tmp.replace(dest_dir / name)
    if keep > 0:
        snaps = sorted(p for p in dest_dir.glob("*.tar.gz") if not p.name.startswith("."))
        for stale in snaps[:-keep]:
            stale.unlink(missing_ok=True)
    return name


def sync_repo(
    repo: Repo,
    timeout: int,
    keep: int,
) -> RepoResult:
    started = time.monotonic()
    result = RepoResult(repo=repo.key)
    # Per-repo env copy: injects a one-shot credential helper so private
    # clones authenticate from $GHA_PAT while URLs stay clean in any output.
    git_env = os.environ.copy()
    git_env["GIT_CONFIG_COUNT"] = "1"
    git_env["GIT_CONFIG_KEY_0"] = "credential.helper"
    git_env["GIT_CONFIG_VALUE_0"] = "!f() { echo username=x-access-token; echo password=$GHA_PAT; }; f"
    try:
        if repo.mirror_path.exists():
            # Only touch config when the URL actually moved (renames): a
            # needless `set-url` rewrites .git/config on every single run.
            current = git(["remote", "get-url", "origin"], cwd=repo.mirror_path, timeout=timeout).strip()
            if current != repo.clone_url:
                git(["remote", "set-url", "origin", repo.clone_url], cwd=repo.mirror_path, timeout=timeout, env=git_env)
            git(["fetch", "--all", "--prune"], cwd=repo.mirror_path, timeout=timeout, env=git_env)
            action = "updated"
        else:
            repo.mirror_path.parent.mkdir(parents=True, exist_ok=True)
            # Clone to a temp dir and rename: an interrupted clone must never
            # leave a half-mirror that every later run trips over.
            tmp = repo.mirror_path.parent / f".{repo.name}.git.tmp-{os.getpid()}"
            shutil.rmtree(tmp, ignore_errors=True)
            git(["clone", "--mirror", repo.clone_url, str(tmp)], timeout=timeout, env=git_env)
            tmp.replace(repo.mirror_path)
            action = "cloned"
        result.refs = len(
            git(["for-each-ref", "--format=%(refname)"], cwd=repo.mirror_path, timeout=timeout).splitlines()
        )
        result.size_bytes = sum(
            p.stat().st_size for p in repo.mirror_path.rglob("*") if p.is_file()
        )
        if keep > 0:
            fp = refs_fingerprint(repo.mirror_path)
            state_file = repo.snapshot_dir / ".state.json"
            state = json.loads(state_file.read_text()) if state_file.exists() else {}
            if state.get("fingerprint") == fp:
                result.snapshot = state.get("snapshot", "")
                result.detail = f"{action}; refs unchanged, snapshot skipped"
            else:
                result.snapshot = snapshot(repo.mirror_path, repo.snapshot_dir, keep)
                state_file.write_text(json.dumps({"fingerprint": fp, "snapshot": result.snapshot}))
                result.detail = f"{action}; snapshot {result.snapshot}"
        else:
            result.detail = f"{action}; snapshots disabled"
    except Exception as exc:  # noqa: BLE001 - one repo failing must not stop the rest
        result.status = "failed"
        result.detail = str(exc)
    result.seconds = round(time.monotonic() - started, 1)
    return result


def prune_orphans(seen: set[str], listing: Listing) -> list[str]:
    """Remove mirrors for repos no longer on GitHub; only after complete listing."""
    removed: list[str] = []
    if not listing.complete:
        LOG.info("repo listing incomplete; skipping orphan prune")
        return removed
    mirrors_root = Path(env("GHA_BACKUP_STATE")) / "mirrors"
    if not mirrors_root.exists():
        return removed
    for owner_dir in mirrors_root.iterdir():
        if not owner_dir.is_dir():
            continue
        for mirror in owner_dir.glob("*.git"):
            key = f"{owner_dir.name}/{mirror.name.removesuffix('.git')}"
            if key not in seen:
                try:
                    subprocess.run(["rm", "-rf", str(mirror)], check=True)
                except subprocess.SubprocessError:
                    LOG.warning("prune failed for %s; leaving in place", key)
                    continue
                removed.append(key)
                LOG.warning("pruned orphan mirror: %s", key)
    return removed


def main() -> None:
    logging.basicConfig(
        stream=sys.stdout,
        format="%(asctime)s %(levelname)s %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%SZ",
        level=logging.INFO,
    )
    token = env("GHA_PAT")

    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--orgs", default=env("GHA_BACKUP_ORGS", "NSXBet"))
    parser.add_argument("--users", default=env("GHA_BACKUP_USERS", "yurifrl"))
    parser.add_argument("--state", default=env("GHA_BACKUP_STATE", "/state"))
    parser.add_argument("--jobs", type=int, default=int(env("GHA_BACKUP_JOBS", "4")))
    parser.add_argument("--snapshots", type=int, default=int(env("GHA_BACKUP_SNAPSHOTS", "5")))
    parser.add_argument("--timeout", type=int, default=int(env("GHA_BACKUP_TIMEOUT", "1800")))
    args = parser.parse_args()

    os.environ["GHA_BACKUP_STATE"] = args.state
    orgs = [o.strip() for o in args.orgs.split(",") if o.strip()]
    users = [u.strip() for u in args.users.split(",") if u.strip()]
    keep = max(0, args.snapshots)
    state_root = Path(args.state)
    state_root.mkdir(parents=True, exist_ok=True)

    started = time.monotonic()
    LOG.info("listing repos (orgs=%s users=%s)", orgs, users)
    listing = list_repos(orgs, users, token)
    LOG.info("found %d repos (listing complete=%s)", len(listing.repos), listing.complete)
    if not listing.repos:
        raise SystemExit("no repos matched the configured orgs/users; refusing to prune, exiting")

    pruned = prune_orphans({r.key for r in listing.repos}, listing)

    results: list[RepoResult] = []
    with ThreadPoolExecutor(max_workers=max(1, args.jobs)) as pool:
        futures = {
            pool.submit(sync_repo, repo, args.timeout, keep): repo for repo in listing.repos
        }
        for future in as_completed(futures):
            result = future.result()
            results.append(result)
            log = LOG.error if result.status == "failed" else LOG.info
            log(
                "%-55s %s in %5.1fs  refs=%d  size=%dMB  %s",
                result.repo,
                result.status,
                result.seconds,
                result.refs,
                result.size_bytes // (1024 * 1024),
                result.detail,
            )

    failed = [r for r in results if r.status == "failed"]
    manifest = {
        "run_ts": now_ts(),
        "orgs": orgs,
        "users": users,
        "repos_total": len(results),
        "repos_failed": len(failed),
        "pruned": pruned,
        "results": [r.__dict__ for r in sorted(results, key=lambda r: r.repo)],
    }
    manifest_path = state_root / "manifest.json"
    tmp_manifest = state_root / ".manifest.json.tmp"
    manifest_ok = True
    try:
        tmp_manifest.write_text(json.dumps(manifest, indent=2))
        tmp_manifest.replace(manifest_path)
    except OSError as exc:
        manifest_ok = False
        LOG.error("manifest write failed: %s", exc)
    LOG.info(
        "done: %d ok, %d failed, %d pruned, %.1fs total -> %s",
        len(results) - len(failed),
        len(failed),
        len(pruned),
        time.monotonic() - started,
        manifest_path if manifest_ok else "(unavailable)",
    )
    if failed or not manifest_ok:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
