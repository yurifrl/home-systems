"""Paths resolved relative to a consumer's config.yaml.

Given a `config.yaml` location, produces the canonical layout:

    <config-dir>/
    ├── config.yaml
    ├── templates/
    └── state/
        ├── assets/
        │   ├── vmlinuz-<arch>
        │   ├── initramfs-<arch>.xz
        │   ├── ipxe.efi
        │   └── boot.ipxe
        ├── ipxe-src/
        ├── configs/
        ├── talosconfig
        ├── kubeconfig
        ├── cache/
        └── logs/
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Paths:
    """Canonical paths derived from the consumer's config.yaml location."""

    config: Path

    @property
    def root(self) -> Path:
        return self.config.parent

    @property
    def templates(self) -> Path:
        return self.root / "templates"

    @property
    def state(self) -> Path:
        return self.root / "state"

    @property
    def assets(self) -> Path:
        return self.state / "assets"

    @property
    def ipxe_src(self) -> Path:
        return self.state / "ipxe-src"

    @property
    def configs(self) -> Path:
        return self.state / "configs"

    @property
    def talosconfig(self) -> Path:
        return self.state / "talosconfig"

    @property
    def kubeconfig(self) -> Path:
        return self.state / "kubeconfig"

    @property
    def cache(self) -> Path:
        return self.state / "cache"

    @property
    def logs(self) -> Path:
        return self.state / "logs"

    @property
    def pending_wipes(self) -> Path:
        return self.state / "pending-wipes.json"

    def ensure_state(self) -> None:
        """mkdir -p every directory we expect under state/."""
        for d in (self.state, self.assets, self.configs, self.cache, self.logs):
            d.mkdir(parents=True, exist_ok=True)
