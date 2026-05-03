"""FastAPI app — localhost dashboard for cluster status and operator actions."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from fastapi import Body, FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse, JSONResponse
from fastapi.staticfiles import StaticFiles

from ..cluster.bootstrap import BootstrapError, bootstrap_node, fetch_kubeconfig
from ..cluster.cert import CertRefreshError, refresh_admin_cert
from ..cluster.status import cluster_status
from ..cluster.wipe import queue_wipe
from ..config import NostosConfig
from ..paths import Paths
from ..registry import RegistryError, get_node

log = logging.getLogger(__name__)

WEB_DIR = Path(__file__).parent
STATIC_DIR = WEB_DIR / "static"


def create_app(cfg: NostosConfig, paths: Paths, *, read_only: bool = False) -> FastAPI:
    app = FastAPI(title="nostos", docs_url=None, redoc_url=None)
    app.state.cfg = cfg
    app.state.paths = paths
    app.state.read_only = read_only

    @app.get("/", response_class=HTMLResponse)
    def index() -> str:
        return (STATIC_DIR / "index.html").read_text()

    @app.get("/api/cluster")
    def cluster() -> dict:
        return {
            "name": cfg.cluster.name,
            "endpoint": cfg.cluster.endpoint,
            "talos_version": cfg.cluster.talos_version,
            "read_only": read_only,
        }

    @app.get("/api/nodes")
    def nodes() -> list[dict]:
        st = cluster_status(cfg, timeout=1.5)
        return [
            {
                "name": n.name,
                "ip": n.ip,
                "role": n.role,
                "ping": n.ping,
                "apid": n.apid,
                "version": n.version,
            }
            for n in st.nodes
        ]

    @app.get("/api/nodes/{name}")
    def node_detail(name: str) -> dict:
        try:
            n = get_node(cfg, name)
        except RegistryError as e:
            raise HTTPException(status_code=404, detail=str(e))
        return {
            "name": name,
            "mac": n.mac,
            "ip": str(n.ip),
            "role": n.role,
            "arch": n.arch,
            "install_disk": n.install_disk,
            "template": n.template,
        }

    def _guard_mutation(confirmation: str | None, name: str) -> None:
        if read_only:
            raise HTTPException(status_code=403, detail="server is read-only")
        if confirmation != name:
            raise HTTPException(
                status_code=400,
                detail=f"confirmation must equal the node name {name!r}",
            )

    @app.post("/api/nodes/{name}/wipe")
    def api_wipe(name: str, body: dict = Body(default={})) -> dict:
        _guard_mutation(body.get("confirmation"), name)
        try:
            n = get_node(cfg, name)
        except RegistryError as e:
            raise HTTPException(status_code=404, detail=str(e))
        queue_wipe(paths, n.mac)
        return {"status": "queued", "mac": n.mac}

    @app.post("/api/nodes/{name}/bootstrap")
    def api_bootstrap(name: str, body: dict = Body(default={})) -> dict:
        _guard_mutation(body.get("confirmation"), name)
        try:
            n = get_node(cfg, name)
        except RegistryError as e:
            raise HTTPException(status_code=404, detail=str(e))
        try:
            bootstrap_node(cfg, paths, n)
            fetch_kubeconfig(paths, n)
        except BootstrapError as e:
            raise HTTPException(status_code=500, detail=str(e))
        return {"status": "bootstrapped"}

    @app.post("/api/nodes/{name}/refresh")
    def api_refresh(name: str, body: dict = Body(default={})) -> dict:
        _guard_mutation(body.get("confirmation"), name)
        try:
            n = get_node(cfg, name)
        except RegistryError as e:
            raise HTTPException(status_code=404, detail=str(e))
        if n.role != "controlplane":
            raise HTTPException(status_code=400, detail="refresh targets a controlplane")
        try:
            refresh_admin_cert(cfg, paths, n)
        except CertRefreshError as e:
            raise HTTPException(status_code=500, detail=str(e))
        return {"status": "refreshed"}

    if STATIC_DIR.is_dir():
        app.mount("/static", StaticFiles(directory=str(STATIC_DIR)), name="static")

    return app


def run_server(
    cfg: NostosConfig,
    paths: Paths,
    *,
    host: str = "127.0.0.1",
    port: int = 8080,
    read_only: bool = False,
) -> None:
    import uvicorn

    app = create_app(cfg, paths, read_only=read_only)
    log.info("starting web dashboard on http://%s:%d (read_only=%s)", host, port, read_only)
    uvicorn.run(app, host=host, port=port, log_level="info")
