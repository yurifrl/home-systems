"""Tests for config discovery."""

from __future__ import annotations

import os
from pathlib import Path

import pytest

from nostos.discovery import ConfigNotFoundError, find_config


@pytest.fixture
def clean_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("NOSTOS_CONFIG", raising=False)


def test_explicit_path_wins(tmp_path: Path, clean_env: None) -> None:
    a = tmp_path / "a.yaml"
    a.write_text("cluster: {}\n")
    cwd_config = tmp_path / "config.yaml"
    cwd_config.write_text("cluster: {}\n")
    assert find_config(explicit=a, cwd=tmp_path) == a.resolve()


def test_explicit_missing_raises(tmp_path: Path, clean_env: None) -> None:
    with pytest.raises(ConfigNotFoundError, match="--config path"):
        find_config(explicit=tmp_path / "nope.yaml", cwd=tmp_path)


def test_env_var_wins_over_cwd(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    env_config = tmp_path / "env.yaml"
    env_config.write_text("cluster: {}\n")
    cwd_config = tmp_path / "config.yaml"
    cwd_config.write_text("cluster: {}\n")
    monkeypatch.setenv("NOSTOS_CONFIG", str(env_config))
    assert find_config(cwd=tmp_path) == env_config.resolve()


def test_env_var_missing_raises(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("NOSTOS_CONFIG", str(tmp_path / "missing.yaml"))
    with pytest.raises(ConfigNotFoundError, match="NOSTOS_CONFIG"):
        find_config(cwd=tmp_path)


def test_cwd_config_discovered(tmp_path: Path, clean_env: None) -> None:
    cwd_config = tmp_path / "config.yaml"
    cwd_config.write_text("cluster: {}\n")
    assert find_config(cwd=tmp_path) == cwd_config.resolve()


def test_walks_up_for_nostos_config(tmp_path: Path, clean_env: None) -> None:
    # nostos/config.yaml at tmp_path root; search from tmp_path/sub/deep/
    nostos_dir = tmp_path / "nostos"
    nostos_dir.mkdir()
    cfg = nostos_dir / "config.yaml"
    cfg.write_text("cluster: {}\n")
    deep = tmp_path / "sub" / "deep"
    deep.mkdir(parents=True)
    assert find_config(cwd=deep) == cfg.resolve()


def test_cwd_config_beats_walk(tmp_path: Path, clean_env: None) -> None:
    # Both tmp_path/config.yaml and tmp_path/nostos/config.yaml exist; cwd wins.
    cwd_config = tmp_path / "config.yaml"
    cwd_config.write_text("cluster: {}\n")
    nostos_dir = tmp_path / "nostos"
    nostos_dir.mkdir()
    (nostos_dir / "config.yaml").write_text("cluster: {}\n")
    assert find_config(cwd=tmp_path) == cwd_config.resolve()


def test_nothing_found_raises(tmp_path: Path, clean_env: None) -> None:
    with pytest.raises(ConfigNotFoundError, match="No config.yaml found"):
        find_config(cwd=tmp_path)
