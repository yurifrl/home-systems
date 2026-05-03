"""Web app tests using FastAPI's TestClient."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

pytest.importorskip("fastapi")

from fastapi.testclient import TestClient  # noqa: E402

from nostos.config import NostosConfig  # noqa: E402
from nostos.paths import Paths  # noqa: E402
from nostos.web.app import create_app  # noqa: E402


BASE_CONFIG = {
    "cluster": {
        "name": "talos-default",
        "endpoint": "https://192.168.68.100:6443",
        "schematic_id": "abc",
    },
    "secrets": {"backend": "env"},
    "nodes": {
        "dell01": {
            "mac": "d0:94:66:d9:eb:a5",
            "ip": "192.168.68.100",
            "role": "controlplane",
            "arch": "amd64",
            "install_disk": "/dev/nvme0n1",
            "template": "dell01.yaml",
        }
    },
}


@pytest.fixture
def client(tmp_path: Path) -> TestClient:
    cfg_path = tmp_path / "config.yaml"
    cfg_path.write_text(yaml.safe_dump(BASE_CONFIG))
    cfg = NostosConfig.load(cfg_path)
    paths = Paths(config=cfg_path)
    paths.ensure_state()
    app = create_app(cfg, paths, read_only=False)
    return TestClient(app)


@pytest.fixture
def readonly_client(tmp_path: Path) -> TestClient:
    cfg_path = tmp_path / "config.yaml"
    cfg_path.write_text(yaml.safe_dump(BASE_CONFIG))
    cfg = NostosConfig.load(cfg_path)
    paths = Paths(config=cfg_path)
    paths.ensure_state()
    app = create_app(cfg, paths, read_only=True)
    return TestClient(app)


def test_index_serves_html(client: TestClient) -> None:
    r = client.get("/")
    assert r.status_code == 200
    assert "nostos" in r.text


def test_cluster_meta(client: TestClient) -> None:
    r = client.get("/api/cluster")
    assert r.status_code == 200
    data = r.json()
    assert data["name"] == "talos-default"
    assert data["read_only"] is False


def test_nodes_list(client: TestClient) -> None:
    r = client.get("/api/nodes")
    assert r.status_code == 200
    rows = r.json()
    assert any(n["name"] == "dell01" for n in rows)


def test_node_detail(client: TestClient) -> None:
    r = client.get("/api/nodes/dell01")
    assert r.status_code == 200
    data = r.json()
    assert data["mac"] == "d0:94:66:d9:eb:a5"


def test_node_detail_404(client: TestClient) -> None:
    assert client.get("/api/nodes/nope").status_code == 404


def test_wipe_requires_confirmation(client: TestClient) -> None:
    r = client.post("/api/nodes/dell01/wipe", json={})
    assert r.status_code == 400


def test_wipe_confirmation_match(client: TestClient, tmp_path: Path) -> None:
    r = client.post("/api/nodes/dell01/wipe", json={"confirmation": "dell01"})
    assert r.status_code == 200
    # state file exists
    assert (tmp_path / "state" / "pending-wipes.json").is_file()


def test_wipe_wrong_confirmation_rejected(client: TestClient) -> None:
    r = client.post("/api/nodes/dell01/wipe", json={"confirmation": "wrong"})
    assert r.status_code == 400


def test_readonly_blocks_mutations(readonly_client: TestClient) -> None:
    r = readonly_client.post("/api/nodes/dell01/wipe", json={"confirmation": "dell01"})
    assert r.status_code == 403
    meta = readonly_client.get("/api/cluster").json()
    assert meta["read_only"] is True
