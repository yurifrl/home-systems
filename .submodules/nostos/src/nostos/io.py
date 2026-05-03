"""Common CLI helpers — error formatting, config loading, output modes."""

from __future__ import annotations

import json
import logging
import sys
from dataclasses import asdict, is_dataclass
from pathlib import Path
from typing import Any

import click
from rich.console import Console

from .config import NostosConfig
from .discovery import ConfigNotFoundError, find_config
from .paths import Paths

console = Console()
err_console = Console(stderr=True)


def configure_logging(debug: bool) -> None:
    level = logging.DEBUG if debug else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
        datefmt="%H:%M:%S",
    )


def load_config(config_path: Path | None) -> tuple[NostosConfig, Paths]:
    """Discover + load config. Exits with a clear message on failure."""
    try:
        path = find_config(explicit=config_path)
    except ConfigNotFoundError as e:
        err_console.print(f"[red]Error:[/red] {e}", soft_wrap=True)
        sys.exit(2)
    try:
        cfg = NostosConfig.load(path)
    except ValueError as e:
        err_console.print(f"[red]Error:[/red] {e}", soft_wrap=True)
        sys.exit(2)
    return cfg, Paths(config=path)


def emit(obj: Any, *, output: str) -> None:
    """Print either rich text or JSON based on --output mode."""
    if output == "json":
        click.echo(json.dumps(_jsonable(obj), indent=2, default=str))
    else:
        console.print(obj)


def _jsonable(obj: Any) -> Any:
    if is_dataclass(obj):
        return asdict(obj)
    if isinstance(obj, dict):
        return {k: _jsonable(v) for k, v in obj.items()}
    if isinstance(obj, list | tuple):
        return [_jsonable(v) for v in obj]
    return obj
