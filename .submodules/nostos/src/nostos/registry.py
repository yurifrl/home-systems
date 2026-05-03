"""Node registry operations — list, add, remove, render, probe.

Wraps `NostosConfig` with side-effectful operations: template rendering
(produces `state/configs/<mac>.yaml`), YAML round-trip writes for
add/remove, reachability/version probing.
"""

from __future__ import annotations

import logging
import shutil
import socket
import subprocess
from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal

import yaml

from .config import NodeConfig, NostosConfig
from .paths import Paths
from .secrets.base import resolve_template
from .secrets.factory import build_backends

log = logging.getLogger(__name__)

Reachability = Literal["up", "down", "refused", "unknown"]


@dataclass
class NodeStatus:
    """Live state for a single node."""

    name: str
    ip: str
    role: str
    ping: Reachability = "unknown"
    apid: Reachability = "unknown"
    version: str | None = None
    errors: list[str] = field(default_factory=list)


class RegistryError(Exception):
    """Raised on registry-level failures (missing template, duplicate, etc.)."""


def list_nodes(cfg: NostosConfig) -> list[tuple[str, NodeConfig]]:
    """Return node registrations in declaration order."""
    return list(cfg.nodes.items())


def get_node(cfg: NostosConfig, name: str) -> NodeConfig:
    """Return a single node config, or raise."""
    try:
        return cfg.nodes[name]
    except KeyError as e:
        known = ", ".join(sorted(cfg.nodes)) or "(none)"
        raise RegistryError(
            f"no such node {name!r}; known: {known}"
        ) from e


def add_node(
    cfg_path: Path, name: str, node: NodeConfig, *, overwrite: bool = False
) -> None:
    """Write a new node entry into config.yaml atomically."""
    raw = yaml.safe_load(cfg_path.read_text()) or {}
    if "nodes" not in raw:
        raw["nodes"] = {}
    if name in raw["nodes"] and not overwrite:
        raise RegistryError(f"node {name!r} already exists in {cfg_path}")
    raw["nodes"][name] = node.model_dump(mode="json", exclude_none=True)
    # Re-validate via pydantic before writing.
    NostosConfig.model_validate(raw)
    _atomic_write_yaml(cfg_path, raw)


def remove_node(cfg_path: Path, name: str) -> None:
    """Remove a node entry from config.yaml atomically."""
    raw = yaml.safe_load(cfg_path.read_text()) or {}
    if name not in raw.get("nodes", {}):
        raise RegistryError(f"no such node {name!r} in {cfg_path}")
    del raw["nodes"][name]
    NostosConfig.model_validate(raw)
    _atomic_write_yaml(cfg_path, raw)


def render_node(
    cfg: NostosConfig,
    paths: Paths,
    name: str,
    *,
    run_validate: bool = True,
) -> Path:
    """Render a machineconfig for `name` into `state/configs/<mac>.yaml`."""
    node = get_node(cfg, name)
    template_path = paths.templates / node.template
    if not template_path.is_file():
        raise RegistryError(
            f"template {template_path} not found for node {name!r}"
        )
    template_text = template_path.read_text()
    backends = build_backends(cfg)
    rendered = resolve_template(template_text, backends)
    paths.configs.mkdir(parents=True, exist_ok=True)
    out_path = paths.configs / f"{node.mac_hyphen}.yaml"
    out_path.write_text(rendered)
    if run_validate:
        _talosctl_validate(out_path)
    return out_path


def probe_node(node: NodeConfig, *, timeout: float = 2.0) -> NodeStatus:
    """Probe ping + apid port 50000 + detect Talos version if reachable."""
    status = NodeStatus(name="", ip=str(node.ip), role=node.role)
    status.ping = _ping(str(node.ip), timeout=timeout)
    status.apid = _tcp_probe(str(node.ip), 50000, timeout=timeout)
    if status.apid == "up":
        status.version = _talosctl_version(str(node.ip))
    return status


# --- internals ---


def _atomic_write_yaml(path: Path, data: dict) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(yaml.safe_dump(data, sort_keys=False))
    tmp.replace(path)


def _talosctl_validate(path: Path) -> None:
    if shutil.which("talosctl") is None:
        log.warning("talosctl not on PATH; skipping machineconfig validation")
        return
    try:
        subprocess.run(
            ["talosctl", "validate", "--config", str(path), "--mode", "metal"],
            check=True,
            capture_output=True,
            text=True,
            timeout=10,
        )
    except subprocess.CalledProcessError as e:
        raise RegistryError(
            f"talosctl validate rejected {path}: {e.stderr.strip() or e.stdout.strip()}"
        ) from e
    except subprocess.TimeoutExpired as e:
        log.warning("talosctl validate timed out on %s", path)
        raise RegistryError("talosctl validate timed out") from e


def _ping(ip: str, timeout: float) -> Reachability:
    # macOS ping -W is milliseconds; Linux -W is seconds. We pick -W in ms (macOS) and fall back.
    import platform
    count_flag = ["-c", "1"]
    if platform.system() == "Darwin":
        wait_flag = ["-W", str(int(timeout * 1000))]
    else:
        wait_flag = ["-W", str(int(timeout))]
    try:
        result = subprocess.run(
            ["ping", *count_flag, *wait_flag, ip],
            capture_output=True,
            text=True,
            timeout=timeout + 1,
        )
        return "up" if result.returncode == 0 else "down"
    except subprocess.TimeoutExpired:
        return "down"
    except FileNotFoundError:
        return "unknown"


def _tcp_probe(ip: str, port: int, timeout: float) -> Reachability:
    try:
        with socket.create_connection((ip, port), timeout=timeout):
            return "up"
    except ConnectionRefusedError:
        return "refused"
    except (OSError, TimeoutError):
        return "down"


def _talosctl_version(ip: str) -> str | None:
    if shutil.which("talosctl") is None:
        return None
    try:
        result = subprocess.run(
            [
                "talosctl",
                "version",
                "--nodes",
                ip,
                "--endpoints",
                ip,
                "--short",
            ],
            capture_output=True,
            text=True,
            timeout=5,
        )
        if result.returncode != 0:
            return None
        for line in result.stdout.splitlines():
            line = line.strip()
            if line.startswith("Server:"):
                return line.split(":", 1)[1].strip() or None
            if line.startswith("Tag:"):
                return line.split(":", 1)[1].strip() or None
        return None
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return None
