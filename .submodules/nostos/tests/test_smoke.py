"""Smoke tests — verify the package imports and exposes the right surface."""

import nostos


def test_version_exposed() -> None:
    assert nostos.__version__ == "0.1.0"


def test_package_importable() -> None:
    # Mere import succeeds; future modules added here as smoke checks.
    assert nostos is not None
