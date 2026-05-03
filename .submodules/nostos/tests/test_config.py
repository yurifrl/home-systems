"""Tests for the configuration schema."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from nostos.config import NodeConfig, NostosConfig


VALID_CONFIG = {
    "cluster": {
        "name": "talos-default",
        "endpoint": "https://192.168.68.100:6443",
        "talos_version": "v1.10.3",
        "schematic_id": "4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b",
    },
    "secrets": {
        "backend": "onepassword",
        "onepassword": {"account": "my.1password.com", "vault": "kubernetes"},
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


def write_yaml(tmp_path: Path, data: dict) -> Path:
    p = tmp_path / "config.yaml"
    p.write_text(yaml.safe_dump(data))
    return p


def test_valid_config_loads(tmp_path: Path) -> None:
    p = write_yaml(tmp_path, VALID_CONFIG)
    cfg = NostosConfig.load(p)
    assert cfg.cluster.name == "talos-default"
    assert "dell01" in cfg.nodes
    assert cfg.nodes["dell01"].mac == "d0:94:66:d9:eb:a5"
    assert cfg.nodes["dell01"].mac_hyphen == "d0-94-66-d9-eb-a5"


def test_mac_uppercase_normalized(tmp_path: Path) -> None:
    data = {**VALID_CONFIG}
    data["nodes"] = {
        "dell01": {**VALID_CONFIG["nodes"]["dell01"], "mac": "D0:94:66:D9:EB:A5"}
    }
    cfg = NostosConfig.load(write_yaml(tmp_path, data))
    assert cfg.nodes["dell01"].mac == "d0:94:66:d9:eb:a5"


def test_invalid_mac_rejected(tmp_path: Path) -> None:
    data = {**VALID_CONFIG}
    data["nodes"] = {
        "dell01": {**VALID_CONFIG["nodes"]["dell01"], "mac": "not-a-mac"}
    }
    with pytest.raises(ValueError, match="invalid MAC"):
        NostosConfig.load(write_yaml(tmp_path, data))


def test_duplicate_mac_rejected(tmp_path: Path) -> None:
    data = {**VALID_CONFIG}
    dell01 = VALID_CONFIG["nodes"]["dell01"]
    data["nodes"] = {
        "dell01": dell01,
        "dell02": {**dell01, "ip": "192.168.68.101"},
    }
    with pytest.raises(ValueError, match="duplicate MAC"):
        NostosConfig.load(write_yaml(tmp_path, data))


def test_missing_secrets_block_rejected(tmp_path: Path) -> None:
    data = {**VALID_CONFIG}
    del data["secrets"]
    with pytest.raises(ValueError):
        NostosConfig.load(write_yaml(tmp_path, data))


def test_onepassword_without_block_rejected(tmp_path: Path) -> None:
    data = {**VALID_CONFIG}
    data["secrets"] = {"backend": "onepassword"}
    with pytest.raises(ValueError, match="onepassword block"):
        NostosConfig.load(write_yaml(tmp_path, data))


def test_invalid_node_name_rejected(tmp_path: Path) -> None:
    data = {**VALID_CONFIG}
    data["nodes"] = {"Dell01!": VALID_CONFIG["nodes"]["dell01"]}
    with pytest.raises(ValueError, match="invalid node name"):
        NostosConfig.load(write_yaml(tmp_path, data))


def test_http_endpoint_rejected(tmp_path: Path) -> None:
    data = {**VALID_CONFIG}
    data["cluster"] = {**VALID_CONFIG["cluster"], "endpoint": "http://foo:6443"}
    with pytest.raises(ValueError, match="must start with https"):
        NostosConfig.load(write_yaml(tmp_path, data))


def test_missing_file() -> None:
    with pytest.raises(FileNotFoundError):
        NostosConfig.load(Path("/nonexistent/config.yaml"))


def test_empty_file(tmp_path: Path) -> None:
    p = tmp_path / "config.yaml"
    p.write_text("")
    with pytest.raises(ValueError, match="is empty"):
        NostosConfig.load(p)


def test_non_mapping_toplevel(tmp_path: Path) -> None:
    p = tmp_path / "config.yaml"
    p.write_text("- just a list\n")
    with pytest.raises(ValueError, match="mapping at the top level"):
        NostosConfig.load(p)


def test_node_config_standalone() -> None:
    n = NodeConfig(
        mac="AA:BB:CC:DD:EE:FF",
        ip="10.0.0.1",
        role="worker",
        arch="arm64",
        install_disk="/dev/sda",
        template="worker.yaml",
    )
    assert n.mac == "aa:bb:cc:dd:ee:ff"
    assert n.mac_hyphen == "aa-bb-cc-dd-ee-ff"
    assert n.role == "worker"
