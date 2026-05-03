"""CLI smoke tests — exercise the click entrypoint end-to-end in-process."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml
from click.testing import CliRunner

from nostos.cli import cli


BASE_CONFIG = {
    "cluster": {
        "name": "talos-default",
        "endpoint": "https://192.168.68.100:6443",
        "talos_version": "v1.10.3",
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
        }
    },
}


@pytest.fixture
def consumer(tmp_path: Path) -> Path:
    cfg = tmp_path / "config.yaml"
    cfg.write_text(yaml.safe_dump(BASE_CONFIG))
    (tmp_path / "templates").mkdir()
    (tmp_path / "templates" / "dell01.yaml").write_text("machine: {}\n")
    return cfg


def test_version() -> None:
    result = CliRunner().invoke(cli, ["--version"])
    assert result.exit_code == 0
    assert "0.1.0" in result.output


def test_help_lists_all_commands() -> None:
    result = CliRunner().invoke(cli, ["--help"])
    assert result.exit_code == 0
    for cmd in [
        "init", "node", "build", "render", "serve", "install",
        "wipe", "bootstrap", "config", "status", "kubeconfig", "nuke", "web",
    ]:
        assert cmd in result.output


def test_init_creates_scaffold(tmp_path: Path) -> None:
    result = CliRunner().invoke(cli, ["init", str(tmp_path / "newproj")])
    assert result.exit_code == 0, result.output
    new = tmp_path / "newproj"
    assert (new / "config.yaml").is_file()
    assert (new / "templates").is_dir()
    assert (new / "state" / ".gitignore").is_file()


def test_init_refuses_overwrite(tmp_path: Path) -> None:
    (tmp_path / "config.yaml").write_text("exists: true\n")
    result = CliRunner().invoke(cli, ["init", str(tmp_path)])
    assert result.exit_code == 1
    assert "already exists" in result.output


def test_render_command_reports_missing_env(consumer: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("TEST_MISSING", raising=False)
    # Point template at an unresolvable env:// URI
    tmpl = consumer.parent / "templates" / "dell01.yaml"
    tmpl.write_text("X=env://TEST_MISSING\n")
    result = CliRunner().invoke(
        cli, ["--config", str(consumer), "render", "dell01", "--no-validate"]
    )
    assert result.exit_code == 1
    assert "env://TEST_MISSING" in result.output


def test_install_prints_cheatsheet(consumer: Path) -> None:
    result = CliRunner().invoke(cli, ["--config", str(consumer), "install", "dell01"])
    assert result.exit_code == 0
    assert "d0:94:66:d9:eb:a5" in result.output
    assert "F12" in result.output
    assert "/dev/nvme0n1" in result.output
    assert "GET /configs/d0-94-66-d9-eb-a5.yaml" in result.output


def test_wipe_queues_node(consumer: Path) -> None:
    result = CliRunner().invoke(cli, ["--config", str(consumer), "wipe", "dell01"])
    assert result.exit_code == 0
    assert "Queued wipe" in result.output
    pending = (consumer.parent / "state" / "pending-wipes.json").read_text()
    assert "d0:94:66:d9:eb:a5" in pending


def test_unknown_command_fails(consumer: Path) -> None:
    result = CliRunner().invoke(cli, ["--config", str(consumer), "nonsense"])
    assert result.exit_code != 0


def test_missing_config_clear_error(tmp_path: Path) -> None:
    result = CliRunner().invoke(
        cli, ["--config", str(tmp_path / "missing.yaml"), "status"]
    )
    assert result.exit_code == 2
    assert "does not exist" in result.output
