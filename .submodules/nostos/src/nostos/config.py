"""Configuration schema for nostos consumers.

A consumer repo declares one `config.yaml` that registers the cluster, the
secrets backend, and every node. This module parses it into validated
pydantic models and fails loud with human-readable errors on schema
violations.
"""

from __future__ import annotations

import re
from collections import Counter
from pathlib import Path
from typing import Literal

import yaml
from pydantic import BaseModel, Field, IPvAnyAddress, field_validator, model_validator

MAC_RE = re.compile(r"^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$")
NODE_NAME_RE = re.compile(r"^[a-z][a-z0-9-]*$")
Arch = Literal["amd64", "arm64"]
Role = Literal["controlplane", "worker"]


class OnepasswordConfig(BaseModel):
    account: str = Field(description="1Password account shorthand, e.g. my.1password.com")
    vault: str = Field(description="Vault name containing the cluster secrets")


class SopsConfig(BaseModel):
    # Placeholder — sops adapter reads file paths directly from URIs.
    age_key_file: str | None = Field(default=None, description="Path to sops age key file")


class SecretsConfig(BaseModel):
    """Selects which secrets backend resolves `<scheme>://...` URIs in templates."""

    backend: Literal["onepassword", "sops", "env", "file"] = "onepassword"
    onepassword: OnepasswordConfig | None = None
    sops: SopsConfig | None = None

    @model_validator(mode="after")
    def _backend_config_present(self) -> SecretsConfig:
        # Require backend-specific config when a non-trivial backend is selected.
        if self.backend == "onepassword" and self.onepassword is None:
            raise ValueError("secrets.backend=onepassword requires secrets.onepassword block")
        return self


class ClusterConfig(BaseModel):
    name: str = Field(description="Talos cluster name, e.g. talos-default")
    endpoint: str = Field(description="Kubernetes apiserver endpoint, e.g. https://192.168.68.100:6443")
    talos_version: str = Field(default="v1.10.3", description="Talos version to install")
    schematic_id: str = Field(description="Image Factory schematic ID (from factory.talos.dev)")

    @field_validator("endpoint")
    @classmethod
    def _endpoint_is_https(cls, v: str) -> str:
        if not v.startswith("https://"):
            raise ValueError("cluster.endpoint must start with https://")
        return v


class NodeConfig(BaseModel):
    """A single declared bare-metal or VM node."""

    mac: str = Field(description="MAC address, colon-separated hex octets")
    ip: IPvAnyAddress = Field(description="Node's IP address (matches machineconfig + reservations)")
    role: Role = Field(description="controlplane or worker")
    arch: Arch = Field(default="amd64", description="CPU architecture")
    install_disk: str = Field(description="Disk device path, e.g. /dev/nvme0n1")
    template: str = Field(description="Template filename (relative to templates/)")

    @field_validator("mac")
    @classmethod
    def _mac_shape(cls, v: str) -> str:
        if not MAC_RE.match(v):
            raise ValueError(
                f"invalid MAC {v!r}: expected six colon-separated hex octets "
                f"(e.g. d0:94:66:d9:eb:a5)"
            )
        return v.lower()

    @property
    def mac_hyphen(self) -> str:
        """MAC in iPXE ${mac:hexhyp} form: d0-94-66-d9-eb-a5 (lowercase)."""
        return self.mac.replace(":", "-")


class NostosConfig(BaseModel):
    """Root config object — the parsed `config.yaml`."""

    cluster: ClusterConfig
    secrets: SecretsConfig
    nodes: dict[str, NodeConfig] = Field(default_factory=dict)

    @field_validator("nodes")
    @classmethod
    def _node_names_valid(cls, v: dict[str, NodeConfig]) -> dict[str, NodeConfig]:
        for name in v:
            if not NODE_NAME_RE.match(name):
                raise ValueError(
                    f"invalid node name {name!r}: must start with a lowercase letter "
                    f"and contain only lowercase letters, digits, and hyphens"
                )
        return v

    @model_validator(mode="after")
    def _no_duplicate_macs(self) -> NostosConfig:
        mac_counter = Counter(n.mac for n in self.nodes.values())
        dupes = [(mac, count) for mac, count in mac_counter.items() if count > 1]
        if dupes:
            by_mac: dict[str, list[str]] = {}
            for name, node in self.nodes.items():
                by_mac.setdefault(node.mac, []).append(name)
            messages = [
                f"  {mac}: {', '.join(sorted(by_mac[mac]))}"
                for mac, _ in dupes
            ]
            raise ValueError(
                "duplicate MAC addresses across nodes:\n" + "\n".join(messages)
            )
        return self

    @classmethod
    def load(cls, path: Path) -> NostosConfig:
        """Load and validate `config.yaml` from a path, with clear errors."""
        if not path.is_file():
            raise FileNotFoundError(f"config file not found: {path}")
        try:
            raw = yaml.safe_load(path.read_text())
        except yaml.YAMLError as e:
            raise ValueError(f"invalid YAML in {path}: {e}") from e
        if raw is None:
            raise ValueError(f"{path} is empty")
        if not isinstance(raw, dict):
            raise ValueError(f"{path} must be a YAML mapping at the top level")
        return cls.model_validate(raw)
