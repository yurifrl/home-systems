"""Discover the consumer's `config.yaml`.

Search order:
1. Explicit path passed to `--config`
2. `$NOSTOS_CONFIG` environment variable
3. `config.yaml` in current working directory
4. `nostos/config.yaml` in current directory or any parent (repo-root style)
"""

from __future__ import annotations

import os
from pathlib import Path


class ConfigNotFoundError(FileNotFoundError):
    """Raised when no config.yaml is discoverable."""


def find_config(explicit: Path | None = None, cwd: Path | None = None) -> Path:
    """Return the path to the active `config.yaml`, or raise."""
    if explicit is not None:
        p = explicit.expanduser().resolve()
        if not p.is_file():
            raise ConfigNotFoundError(f"--config path does not exist: {p}")
        return p

    env = os.environ.get("NOSTOS_CONFIG")
    if env:
        p = Path(env).expanduser().resolve()
        if not p.is_file():
            raise ConfigNotFoundError(f"$NOSTOS_CONFIG points to a missing file: {p}")
        return p

    start = (cwd or Path.cwd()).resolve()

    # 3. cwd/config.yaml
    cwd_config = start / "config.yaml"
    if cwd_config.is_file():
        return cwd_config

    # 4. walk up looking for nostos/config.yaml
    current = start
    while True:
        candidate = current / "nostos" / "config.yaml"
        if candidate.is_file():
            return candidate
        if current.parent == current:  # filesystem root
            break
        current = current.parent

    raise ConfigNotFoundError(
        "No config.yaml found. Checked --config flag, $NOSTOS_CONFIG, "
        f"{cwd_config}, and nostos/config.yaml in every parent of {start}. "
        "Run `nostos init` to create one."
    )
