"""Cluster-layer tests — wipe registry + cert-refresh path handling.

Bootstrap and status tests require a real Talos cluster and are covered
by integration/manual verification in section 11.
"""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from nostos.cluster.bootstrap import BootstrapError, bootstrap_node
from nostos.cluster.cert import CertRefreshError, refresh_admin_cert
from nostos.cluster.wipe import consume_wipe, pending_wipes, queue_wipe
from nostos.config import NodeConfig, NostosConfig
from nostos.paths import Paths


BASE_CONFIG = {
    "cluster": {
        "name": "talos-default",
        "endpoint": "https://192.168.68.100:6443",
        "schematic_id": "abc",
    },
    "secrets": {"backend": "env"},
    "nodes": {
        "dell01": {
            "mac": "d0:94:66:d9:eb:a5",
            "ip": "192.168.68.100",
            "role": "controlplane",
            "arch": "amd64",
            "install_disk": "/dev/nvme0n1",
            "template": "dell01.yaml",
        },
        "tp1": {
            "mac": "aa:bb:cc:dd:ee:ff",
            "ip": "192.168.68.107",
            "role": "worker",
            "arch": "arm64",
            "install_disk": "/dev/mmcblk0",
            "template": "tp1.yaml",
        },
    },
}


@pytest.fixture
def paths(tmp_path: Path) -> Paths:
    cfg = tmp_path / "config.yaml"
    cfg.write_text(yaml.safe_dump(BASE_CONFIG))
    p = Paths(config=cfg)
    p.ensure_state()
    return p


def test_wipe_queue_roundtrip(paths: Paths) -> None:
    assert pending_wipes(paths) == set()
    queue_wipe(paths, "d0:94:66:d9:eb:a5")
    assert "d0:94:66:d9:eb:a5" in pending_wipes(paths)
    consumed = consume_wipe(paths, "D0:94:66:D9:EB:A5")  # case-insensitive
    assert consumed is True
    assert pending_wipes(paths) == set()
    # Second consume returns False (already consumed)
    assert consume_wipe(paths, "d0:94:66:d9:eb:a5") is False


def test_bootstrap_rejects_worker(paths: Paths) -> None:
    cfg = NostosConfig.load(paths.config)
    worker = cfg.nodes["tp1"]
    with pytest.raises(BootstrapError, match="controlplane"):
        bootstrap_node(cfg, paths, worker)


def test_cert_refresh_needs_rendered_machineconfig(paths: Paths) -> None:
    cfg = NostosConfig.load(paths.config)
    controlplane = cfg.nodes["dell01"]
    with pytest.raises(CertRefreshError, match="could not extract CA"):
        refresh_admin_cert(cfg, paths, controlplane)
