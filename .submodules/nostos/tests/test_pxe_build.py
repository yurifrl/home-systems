"""PXE build tests — render logic + embed script; integration tests are skipped
unless Docker is available.
"""

from __future__ import annotations

import os
import shutil
from pathlib import Path

import pytest
import yaml

from nostos.config import NostosConfig
from nostos.paths import Paths
from nostos.pxe.build import (
    EMBED_IPXE,
    IPXE_MAX_SIZE_BYTES,
    BuildError,
    build_all,
    render_boot_ipxe,
)


BASE_CONFIG = {
    "cluster": {
        "name": "talos-default",
        "endpoint": "https://192.168.68.100:6443",
        "talos_version": "v1.10.3",
        "schematic_id": "4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b",
    },
    "secrets": {"backend": "env"},
    "nodes": {},
}


@pytest.fixture
def consumer(tmp_path: Path) -> Paths:
    cfg = tmp_path / "config.yaml"
    cfg.write_text(yaml.safe_dump(BASE_CONFIG))
    p = Paths(config=cfg)
    p.ensure_state()
    return p


def test_render_boot_ipxe_uses_next_server(consumer: Paths) -> None:
    cfg = NostosConfig.load(consumer.config)
    out = render_boot_ipxe(cfg, consumer, arch="amd64")
    text = out.read_text()
    assert "${next-server}" in text
    # no hardcoded IPv4 addresses
    import re
    assert not re.search(r"\b\d{1,3}(\.\d{1,3}){3}\b", text), "boot.ipxe must not hardcode an IP"
    assert "vmlinuz-amd64" in text
    assert "initramfs-amd64.xz" in text
    assert "talos.platform=metal" in text
    assert "talos.config=" in text
    assert "v1.10.3" in text


def test_render_boot_ipxe_passes_extra_args(consumer: Paths) -> None:
    cfg = NostosConfig.load(consumer.config)
    out = render_boot_ipxe(cfg, consumer, extra_kernel_args="talos.experimental.wipe=system")
    assert "talos.experimental.wipe=system" in out.read_text()


def test_embed_script_has_retry_loop() -> None:
    assert "retry_dhcp" in EMBED_IPXE
    assert "isset ${filename}" in EMBED_IPXE
    assert "chain ${filename}" in EMBED_IPXE


def test_build_requires_docker_if_missing_ipxe(
    consumer: Paths, monkeypatch: pytest.MonkeyPatch
) -> None:
    # Pretend docker isn't on PATH. Test build_ipxe directly (skip network).
    from nostos.pxe.build import build_ipxe

    real_which = shutil.which

    def fake_which(name: str) -> str | None:
        return None if name == "docker" else real_which(name)

    monkeypatch.setattr("nostos.pxe.build.shutil.which", fake_which)
    with pytest.raises(BuildError, match="Docker is required"):
        build_ipxe(consumer)


@pytest.mark.skipif(
    shutil.which("docker") is None or os.environ.get("NOSTOS_SKIP_INTEGRATION"),
    reason="Docker not available or integration tests disabled",
)
def test_integration_build_produces_small_ipxe(consumer: Paths) -> None:
    """Integration test — requires Docker + network. Slow."""
    cfg = NostosConfig.load(consumer.config)
    build_all(cfg, consumer)
    ipxe = consumer.assets / "ipxe.efi"
    assert ipxe.is_file()
    assert ipxe.stat().st_size < IPXE_MAX_SIZE_BYTES
