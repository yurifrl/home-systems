"""Admin client cert regeneration against the existing cluster CA.

Solves the expired-talosconfig trap: when the admin client cert rots, you
can't talk to the cluster to rotate a new one via `talosctl config new`.
Here, we extract the CA from a rendered machineconfig (or directly from
the secrets backend) and sign a fresh admin cert offline.

Uses Python's base64 module (not the macOS `base64` CLI, which emits CRLF
and has bitten this project before).
"""

from __future__ import annotations

import base64
import logging
import shutil
import subprocess
import tempfile
from pathlib import Path

import yaml

from ..config import NodeConfig, NostosConfig
from ..paths import Paths

log = logging.getLogger(__name__)

DEFAULT_HOURS = 876_000  # ~100 years


class CertRefreshError(RuntimeError):
    pass


def refresh_admin_cert(
    cfg: NostosConfig,
    paths: Paths,
    controlplane_node: NodeConfig,
    *,
    hours: int = DEFAULT_HOURS,
    ca_cert: bytes | None = None,
    ca_key: bytes | None = None,
) -> Path:
    """Regenerate admin client cert against an existing CA.

    CA bytes may be passed explicitly or extracted from a rendered
    machineconfig for the target controlplane.
    """
    if shutil.which("talosctl") is None:
        raise CertRefreshError(
            "talosctl not found. Install: brew install siderolabs/tap/talosctl"
        )

    ca_cert_bytes, ca_key_bytes = _resolve_ca(
        cfg, paths, controlplane_node, ca_cert=ca_cert, ca_key=ca_key
    )

    with tempfile.TemporaryDirectory(prefix="nostos-cert-") as tmp:
        work = Path(tmp)
        (work / "ca.crt").write_bytes(ca_cert_bytes)
        (work / "ca.key").write_bytes(ca_key_bytes)

        # 1. Generate admin private key
        _run_talosctl(["gen", "key", "--name", "admin"], cwd=work, force_ok=False)

        # 2. Generate CSR for os:admin role (talosctl requires --ip)
        _run_talosctl(
            ["gen", "csr", "--key", "admin.key", "--roles", "os:admin", "--ip", "127.0.0.1"],
            cwd=work,
            force_ok=False,
        )

        # 3. Sign with CA
        _run_talosctl(
            [
                "gen", "crt",
                "--name", "admin",
                "--ca", "ca",
                "--csr", "admin.csr",
                "--hours", str(hours),
            ],
            cwd=work,
            force_ok=True,
        )

        # Assemble talosconfig
        ca_b64 = _b64(ca_cert_bytes)
        crt_b64 = _b64((work / "admin.crt").read_bytes())
        key_b64 = _b64((work / "admin.key").read_bytes())

        endpoint_ip = str(controlplane_node.ip)
        talosconfig = {
            "context": cfg.cluster.name,
            "contexts": {
                cfg.cluster.name: {
                    "endpoints": [endpoint_ip],
                    "nodes": [endpoint_ip],
                    "ca": ca_b64,
                    "crt": crt_b64,
                    "key": key_b64,
                }
            },
        }
        paths.state.mkdir(parents=True, exist_ok=True)
        paths.talosconfig.write_text(yaml.safe_dump(talosconfig, sort_keys=False))
        paths.talosconfig.chmod(0o600)
        log.info("wrote %s (valid %d hours)", paths.talosconfig, hours)
        return paths.talosconfig


# --- internals ---


def _b64(data: bytes) -> str:
    """Base64 encode without line wrapping. Python's module, NOT the macOS CLI."""
    return base64.b64encode(data).decode("ascii")


def _run_talosctl(args: list[str], *, cwd: Path, force_ok: bool) -> None:
    cmd = ["talosctl"] + args + (["--force"] if force_ok else [])
    try:
        subprocess.run(cmd, check=True, cwd=str(cwd), capture_output=True, text=True, timeout=10)
    except subprocess.CalledProcessError as e:
        raise CertRefreshError(
            f"talosctl {' '.join(args)} failed: {e.stderr.strip() or e.stdout.strip()}"
        ) from e


def _resolve_ca(
    cfg: NostosConfig,
    paths: Paths,
    controlplane_node: NodeConfig,
    *,
    ca_cert: bytes | None,
    ca_key: bytes | None,
) -> tuple[bytes, bytes]:
    """Return raw PEM bytes for cluster CA cert + key."""
    if ca_cert and ca_key:
        return ca_cert, ca_key

    # Prefer a rendered machineconfig already on disk.
    rendered = paths.configs / f"{controlplane_node.mac_hyphen}.yaml"
    if rendered.is_file():
        docs = list(yaml.safe_load_all(rendered.read_text()))
        machine = docs[0].get("machine", {}) if docs else {}
        ca = machine.get("ca") or {}
        if ca.get("crt") and ca.get("key"):
            try:
                return base64.b64decode(ca["crt"]), base64.b64decode(ca["key"])
            except Exception as e:
                raise CertRefreshError(f"CA fields in {rendered} are not valid base64: {e}") from e

    raise CertRefreshError(
        f"could not extract CA from {rendered}. Run `nostos render {controlplane_node.mac}` "
        f"for a controlplane node first, or pass --ca-cert / --ca-key explicitly."
    )
