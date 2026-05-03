"""One-shot disk-wipe flag registry.

When the operator marks a node for wipe via `nostos wipe <name>`, we add
its MAC to a persisted list. The PXE serve layer consumes this list when
composing the per-MAC `boot.ipxe` content: for every wipe-marked MAC, the
kernel cmdline gains `talos.experimental.wipe=system`. After the node
successfully reinstalls, the flag is cleared so subsequent boots don't
loop forever (the infamous trap we hit this session).
"""

from __future__ import annotations

import json
from pathlib import Path

from ..paths import Paths


def _load(paths: Paths) -> dict[str, dict]:
    if not paths.pending_wipes.is_file():
        return {}
    try:
        return json.loads(paths.pending_wipes.read_text()) or {}
    except json.JSONDecodeError:
        return {}


def _save(paths: Paths, data: dict[str, dict]) -> None:
    paths.pending_wipes.parent.mkdir(parents=True, exist_ok=True)
    paths.pending_wipes.write_text(json.dumps(data, indent=2))


def queue_wipe(paths: Paths, mac: str) -> None:
    """Mark a node for one-shot wipe on next PXE boot."""
    data = _load(paths)
    data[mac.lower()] = {"pending": True}
    _save(paths, data)


def consume_wipe(paths: Paths, mac: str) -> bool:
    """Return True if the node was pending wipe; clear the flag either way."""
    data = _load(paths)
    was_pending = data.pop(mac.lower(), None) is not None
    _save(paths, data)
    return was_pending


def pending_wipes(paths: Paths) -> set[str]:
    """Return the set of MACs currently marked for wipe."""
    return set(_load(paths).keys())
