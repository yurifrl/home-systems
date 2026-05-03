"""PXE serve tests — pure-logic checks. Actual dnsmasq/sudo integration is
out of scope for CI."""

from __future__ import annotations

import os
from pathlib import Path

import pytest
import yaml

from nostos.config import NostosConfig
from nostos.paths import Paths
from nostos.pxe.serve import NetworkInfo, Server, ServeError


BASE_CONFIG = {
    "cluster": {
        "name": "talos-default",
        "endpoint": "https://192.168.68.100:6443",
        "schematic_id": "abc",
    },
    "secrets": {"backend": "env"},
    "nodes": {},
}


@pytest.fixture
def paths(tmp_path: Path) -> Paths:
    cfg = tmp_path / "config.yaml"
    cfg.write_text(yaml.safe_dump(BASE_CONFIG))
    p = Paths(config=cfg)
    p.ensure_state()
    return p


def test_preflight_fails_without_assets(paths: Paths) -> None:
    s = Server(paths, iface="en5")
    with pytest.raises(ServeError, match="Run `nostos build`"):
        s.preflight()


def test_preflight_succeeds_with_assets(
    paths: Paths, monkeypatch: pytest.MonkeyPatch
) -> None:
    (paths.assets / "ipxe.efi").write_bytes(b"x")
    (paths.assets / "boot.ipxe").write_text("#!ipxe\n")

    # Mock the dnsmasq presence check.
    monkeypatch.setattr(
        "nostos.pxe.serve.shutil.which", lambda name: "/usr/local/sbin/" + name
    )
    s = Server(paths, iface="en5")
    info = s.preflight()
    assert info.interface == "en5"


def test_stage_tftp(paths: Paths, tmp_path: Path) -> None:
    (paths.assets / "ipxe.efi").write_bytes(b"binary-data")
    tftp_root = tmp_path / "tftp"
    s = Server(paths, tftp_root=str(tftp_root))
    s.stage_tftp()
    staged = tftp_root / "ipxe.efi"
    assert staged.is_file()
    assert staged.read_bytes() == b"binary-data"
    # Must be world-readable.
    assert staged.stat().st_mode & 0o644 == 0o644
