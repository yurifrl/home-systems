"""Registry tests using a fixture consumer repo."""

from __future__ import annotations

from pathlib import Path
from unittest.mock import patch

import pytest
import yaml

from nostos.config import NodeConfig, NostosConfig
from nostos.paths import Paths
from nostos.registry import (
    RegistryError,
    add_node,
    get_node,
    list_nodes,
    probe_node,
    remove_node,
    render_node,
)


BASE_CONFIG = {
    "cluster": {
        "name": "talos-default",
        "endpoint": "https://192.168.68.100:6443",
        "schematic_id": "abc",
    },
    "secrets": {
        "backend": "env",
    },
    "nodes": {
        "dell01": {
            "mac": "d0:94:66:d9:eb:a5",
            "ip": "192.168.68.100",
            "role": "controlplane",
            "arch": "amd64",
            "install_disk": "/dev/nvme0n1",
            "template": "dell01.yaml",
        }
    },
}


@pytest.fixture
def consumer_repo(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    """A tmp consumer repo with config.yaml + templates/dell01.yaml."""
    cfg_path = tmp_path / "config.yaml"
    cfg_path.write_text(yaml.safe_dump(BASE_CONFIG))
    tmpl = tmp_path / "templates"
    tmpl.mkdir()
    (tmpl / "dell01.yaml").write_text(
        "machine:\n"
        "  type: controlplane\n"
        "  network:\n"
        "    hostname: dell01\n"
        "---\n"
        "apiVersion: v1alpha1\n"
        "kind: ExtensionServiceConfig\n"
        "name: tailscale\n"
        "environment:\n"
        "  - TS_AUTHKEY=env://TEST_TS_AUTHKEY\n"
    )
    monkeypatch.setenv("TEST_TS_AUTHKEY", "tskey-fake")
    return cfg_path


def test_list_nodes(consumer_repo: Path) -> None:
    cfg = NostosConfig.load(consumer_repo)
    nodes = list_nodes(cfg)
    assert [n[0] for n in nodes] == ["dell01"]


def test_get_node_raises_on_unknown(consumer_repo: Path) -> None:
    cfg = NostosConfig.load(consumer_repo)
    with pytest.raises(RegistryError, match="no such node"):
        get_node(cfg, "nope")


def test_add_node(consumer_repo: Path) -> None:
    new = NodeConfig(
        mac="aa:bb:cc:dd:ee:ff",
        ip="192.168.68.107",
        role="worker",
        arch="arm64",
        install_disk="/dev/mmcblk0",
        template="tp1.yaml",
    )
    add_node(consumer_repo, "tp1", new)
    cfg = NostosConfig.load(consumer_repo)
    assert "tp1" in cfg.nodes
    assert cfg.nodes["tp1"].ip.exploded == "192.168.68.107"


def test_add_duplicate_rejected(consumer_repo: Path) -> None:
    new = NodeConfig(
        mac="aa:bb:cc:dd:ee:ff",
        ip="192.168.68.107",
        role="worker",
        arch="arm64",
        install_disk="/dev/mmcblk0",
        template="tp1.yaml",
    )
    with pytest.raises(RegistryError, match="already exists"):
        add_node(consumer_repo, "dell01", new)


def test_remove_node(consumer_repo: Path) -> None:
    add_node(
        consumer_repo,
        "tp1",
        NodeConfig(
            mac="aa:bb:cc:dd:ee:ff",
            ip="192.168.68.107",
            role="worker",
            arch="arm64",
            install_disk="/dev/mmcblk0",
            template="tp1.yaml",
        ),
    )
    remove_node(consumer_repo, "tp1")
    cfg = NostosConfig.load(consumer_repo)
    assert "tp1" not in cfg.nodes


def test_render_node_produces_mac_filename(consumer_repo: Path) -> None:
    cfg = NostosConfig.load(consumer_repo)
    paths = Paths(config=consumer_repo)
    # Skip talosctl validate (may not be installed, or may reject the tiny fixture).
    out = render_node(cfg, paths, "dell01", run_validate=False)
    assert out.name == "d0-94-66-d9-eb-a5.yaml"
    rendered = out.read_text()
    assert "TS_AUTHKEY=tskey-fake" in rendered
    assert "env://" not in rendered  # URI was resolved


def test_render_missing_template_fails(consumer_repo: Path) -> None:
    cfg = NostosConfig.load(consumer_repo)
    paths = Paths(config=consumer_repo)
    # Delete the template file.
    (paths.templates / "dell01.yaml").unlink()
    with pytest.raises(RegistryError, match="template .* not found"):
        render_node(cfg, paths, "dell01", run_validate=False)


def test_probe_node_on_loopback() -> None:
    node = NodeConfig(
        mac="aa:bb:cc:dd:ee:ff",
        ip="127.0.0.1",
        role="controlplane",
        install_disk="/dev/nvme0n1",
        template="x.yaml",
    )
    status = probe_node(node, timeout=1.0)
    assert status.ping in ("up", "unknown", "down")  # depends on env
    assert status.apid in ("refused", "down", "up")  # 50000 probably closed
