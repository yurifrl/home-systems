"""talosctl bootstrap + wait-for-ready + fetch kubeconfig."""

from __future__ import annotations

import logging
import shutil
import subprocess
import time
from pathlib import Path

from ..config import NodeConfig, NostosConfig
from ..paths import Paths

log = logging.getLogger(__name__)


class BootstrapError(RuntimeError):
    pass


def _require_talosctl() -> None:
    if shutil.which("talosctl") is None:
        raise BootstrapError(
            "talosctl not found. Install: brew install siderolabs/tap/talosctl"
        )


def bootstrap_node(
    cfg: NostosConfig, paths: Paths, node: NodeConfig, *, timeout: float = 300.0
) -> None:
    """Run talosctl bootstrap; idempotent on already-bootstrapped cluster."""
    _require_talosctl()
    if node.role != "controlplane":
        raise BootstrapError(
            f"bootstrap targets controlplane nodes; node role is {node.role!r}"
        )

    if not paths.talosconfig.is_file():
        raise BootstrapError(
            f"no talosconfig at {paths.talosconfig}. Run `nostos config refresh` first."
        )

    ip = str(node.ip)
    log.info("bootstrapping etcd on %s", ip)

    try:
        result = subprocess.run(
            [
                "talosctl",
                "--talosconfig",
                str(paths.talosconfig),
                "--nodes",
                ip,
                "--endpoints",
                ip,
                "bootstrap",
            ],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except subprocess.TimeoutExpired as e:
        raise BootstrapError(f"talosctl bootstrap timed out after 30s") from e

    if result.returncode != 0:
        # Idempotent-success: "AlreadyExists" from Talos means etcd is up.
        if "AlreadyExists" in result.stderr or "already bootstrapped" in result.stderr.lower():
            log.info("cluster already bootstrapped on %s — continuing", ip)
        else:
            raise BootstrapError(
                f"talosctl bootstrap failed: {result.stderr.strip() or result.stdout.strip()}"
            )

    wait_for_etcd(cfg, paths, node, timeout=timeout)


def wait_for_etcd(
    cfg: NostosConfig, paths: Paths, node: NodeConfig, *, timeout: float = 300.0
) -> None:
    """Poll talosctl service etcd until Running/OK or timeout."""
    _require_talosctl()
    ip = str(node.ip)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            r = subprocess.run(
                [
                    "talosctl",
                    "--talosconfig",
                    str(paths.talosconfig),
                    "--nodes",
                    ip,
                    "--endpoints",
                    ip,
                    "service",
                    "etcd",
                ],
                capture_output=True,
                text=True,
                timeout=5,
            )
            if r.returncode == 0 and "Running" in r.stdout and "OK" in r.stdout:
                log.info("etcd healthy on %s", ip)
                return
        except subprocess.TimeoutExpired:
            pass
        time.sleep(3)
    raise BootstrapError(f"etcd did not become healthy on {ip} within {timeout}s")


def fetch_kubeconfig(paths: Paths, node: NodeConfig, *, force: bool = True) -> Path:
    """Save cluster kubeconfig to state/kubeconfig."""
    _require_talosctl()
    ip = str(node.ip)
    args = [
        "talosctl",
        "--talosconfig",
        str(paths.talosconfig),
        "--nodes",
        ip,
        "--endpoints",
        ip,
        "kubeconfig",
        str(paths.kubeconfig),
    ]
    if force:
        args.append("--force")
    try:
        subprocess.run(args, check=True, capture_output=True, text=True, timeout=15)
    except subprocess.CalledProcessError as e:
        raise BootstrapError(
            f"kubeconfig fetch failed: {e.stderr.strip() or e.stdout.strip()}"
        ) from e
    return paths.kubeconfig
