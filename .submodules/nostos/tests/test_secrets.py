"""Tests for the secrets backend layer.

Uses a mock backend so real secrets are never touched.
"""

from __future__ import annotations

import logging
import os
from typing import ClassVar
from unittest.mock import patch

import pytest

from nostos.secrets.base import SecretResolutionError, find_uris, resolve_template
from nostos.secrets.env import EnvBackend
from nostos.secrets.file import FileBackend


class MockOpBackend:
    scheme: ClassVar[str] = "op"

    def __init__(self) -> None:
        self.vault = {
            "op://kubernetes/talos/TS_AUTHKEY": "tskey-auth-example",
            "op://kubernetes/talos/MACHINE_CA_CRT": "LS0tLS1CRUdJTi",
        }
        self.validate_called = False

    def validate(self) -> None:
        self.validate_called = True

    def resolve(self, uri: str) -> str:
        try:
            return self.vault[uri]
        except KeyError as e:
            raise SecretResolutionError(uri, "not in mock vault") from e


def test_find_uris_matches_registered_schemes() -> None:
    # The package-level __init__ registers op/sops/env/file.
    text = """
    TS_AUTHKEY=op://kubernetes/talos/TS_AUTHKEY
    endpoint: https://192.168.68.100:6443
    CA=env://MY_CA
    RAW=file:///tmp/ca.crt
    """
    uris = find_uris(text)
    assert "op://kubernetes/talos/TS_AUTHKEY" in uris
    assert "env://MY_CA" in uris
    assert "file:///tmp/ca.crt" in uris
    # https:// is not a registered scheme, so it's NOT returned.
    assert not any(u.startswith("https://") for u in uris)


def test_resolve_template_replaces_uris() -> None:
    backends = {"op": MockOpBackend()}
    text = "TS_AUTHKEY=op://kubernetes/talos/TS_AUTHKEY\n"
    out = resolve_template(text, backends)
    assert out == "TS_AUTHKEY=tskey-auth-example\n"


def test_resolve_template_preserves_https() -> None:
    # https:// is not a registered scheme → passes through unchanged.
    backends = {"op": MockOpBackend()}
    text = "endpoint: https://192.168.68.100:6443\n"
    assert resolve_template(text, backends) == text


def test_resolve_template_preserves_unregistered_scheme() -> None:
    # A scheme with no backend in the map passes through unchanged.
    backends: dict = {}
    text = "SECRET=op://foo/bar\n"
    # Without a registered op backend in the map, the URI is left literal.
    # (find_uris ignores it because no backend is registered at the package
    # level either, unless the secrets __init__ did so — see below.)
    # Force the behavior: even if find_uris would return it, resolve_template
    # checks the backends dict.
    assert resolve_template(text, backends) == text


def test_resolve_template_raises_on_failure() -> None:
    backends = {"op": MockOpBackend()}
    with pytest.raises(SecretResolutionError, match="not in mock vault"):
        resolve_template("X=op://missing/item/field\n", backends)


def test_resolve_template_error_does_not_leak_value() -> None:
    backends = {"op": MockOpBackend()}
    try:
        resolve_template("X=op://missing/item/field\n", backends)
    except SecretResolutionError as e:
        assert "op://missing/item/field" in str(e)
        assert "tskey-auth-example" not in str(e)


def test_env_backend(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MY_SECRET", "hunter2")
    b = EnvBackend()
    assert b.resolve("env://MY_SECRET") == "hunter2"


def test_env_backend_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("MISSING_X", raising=False)
    with pytest.raises(SecretResolutionError, match="is not set"):
        EnvBackend().resolve("env://MISSING_X")


def test_env_backend_empty_name() -> None:
    with pytest.raises(SecretResolutionError, match="empty variable name"):
        EnvBackend().resolve("env://")


def test_file_backend_reads_file(tmp_path) -> None:
    p = tmp_path / "ca.crt"
    p.write_text("hello\n\n")
    out = FileBackend().resolve(f"file://{p}")
    assert out == "hello"


def test_file_backend_missing(tmp_path) -> None:
    with pytest.raises(SecretResolutionError, match="file not found"):
        FileBackend().resolve(f"file://{tmp_path}/nope")


def test_resolve_does_not_log_values(caplog: pytest.LogCaptureFixture) -> None:
    """Regression: resolved secrets must never appear in logs."""
    caplog.set_level(logging.DEBUG)
    backends = {"op": MockOpBackend()}
    out = resolve_template("X=op://kubernetes/talos/TS_AUTHKEY\n", backends)
    assert "tskey-auth-example" in out  # it's in the output, where it belongs
    # But never in logs.
    for rec in caplog.records:
        assert "tskey-auth-example" not in rec.getMessage()
