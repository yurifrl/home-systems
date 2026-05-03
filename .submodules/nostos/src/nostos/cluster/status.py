"""Per-node reachability + version + kubelet status report."""

from __future__ import annotations

from dataclasses import dataclass, field

from ..config import NostosConfig
from ..registry import NodeStatus, probe_node


@dataclass
class ClusterStatus:
    nodes: list[NodeStatus] = field(default_factory=list)


def cluster_status(cfg: NostosConfig, *, timeout: float = 2.0) -> ClusterStatus:
    status = ClusterStatus()
    for name, node in cfg.nodes.items():
        probe = probe_node(node, timeout=timeout)
        probe.name = name
        status.nodes.append(probe)
    return status
