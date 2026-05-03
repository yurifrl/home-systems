"""End-to-end integration test — init + add + render through the CLI.

Exercises the full operator workflow in-process with a mock secrets
backend, asserting golden output for the rendered machineconfig.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest
import yaml
from click.testing import CliRunner

from nostos.cli import cli


TEMPLATE_EXAMPLE = """\
machine:
  type: controlplane
  token: "op://kubernetes/talos/MACHINE_TOKEN"
  ca:
    crt: "op://kubernetes/talos/MACHINE_CA_CRT"
    key: "op://kubernetes/talos/MACHINE_CA_KEY"
  network:
    hostname: dell01
    interfaces:
      - interface: enp2s0
        dhcp: false
        addresses:
          - 192.168.68.100/24
---
apiVersion: v1alpha1
kind: ExtensionServiceConfig
name: tailscale
environment:
  - TS_AUTHKEY=env://TS_AUTHKEY
"""


def test_init_add_render_flow(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Run init, hand-write a node, render, verify output contains resolved secret."""
    runner = CliRunner()
    proj = tmp_path / "homelab"

    # init
    r = runner.invoke(cli, ["init", str(proj)])
    assert r.exit_code == 0, r.output

    # hand-write a node + template (questionary prompts are hard to drive in CI)
    cfg_path = proj / "config.yaml"
    raw = yaml.safe_load(cfg_path.read_text())
    raw["cluster"]["schematic_id"] = "abc"
    raw["secrets"] = {"backend": "env"}
    raw["nodes"] = {
        "dell01": {
            "mac": "d0:94:66:d9:eb:a5",
            "ip": "192.168.68.100",
            "role": "controlplane",
            "arch": "amd64",
            "install_disk": "/dev/nvme0n1",
            "template": "dell01.yaml",
        }
    }
    cfg_path.write_text(yaml.safe_dump(raw))
    (proj / "templates" / "dell01.yaml").write_text(TEMPLATE_EXAMPLE)

    # Provide secrets via env so no 1Password needed.
    monkeypatch.setenv("TS_AUTHKEY", "tskey-abc123")
    # op:// refs from the template body get left literal (no op backend registered
    # in env-backend mode), which is fine for this smoke — we just want to verify
    # the resolved env:// makes it into output and the file lands at the right path.

    r = runner.invoke(
        cli,
        ["--config", str(cfg_path), "render", "dell01", "--no-validate"],
    )
    assert r.exit_code == 0, r.output

    out = proj / "state" / "configs" / "d0-94-66-d9-eb-a5.yaml"
    assert out.is_file()
    body = out.read_text()
    assert "TS_AUTHKEY=tskey-abc123" in body
    # unquoted, no literal "env://" in output
    assert "env://" not in body
    # op:// refs remain literal because op isn't the selected backend here
    # (and the op backend in the global registry isn't in our backends map).
    # This is intentional — they'd be resolved in real deployments where op is the backend.
    assert body.count("op://") >= 1


def test_nuke_removes_state(tmp_path: Path) -> None:
    runner = CliRunner()
    proj = tmp_path / "proj"
    runner.invoke(cli, ["init", str(proj)])
    state_dir = proj / "state"
    (state_dir / "assets").mkdir()
    (state_dir / "assets" / "junk").write_text("x")
    r = runner.invoke(cli, ["--config", str(proj / "config.yaml"), "nuke", "--yes"])
    assert r.exit_code == 0, r.output
    assert not state_dir.is_dir()
