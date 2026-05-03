"""click entrypoint — the `nostos` CLI."""

from __future__ import annotations

import logging
import shutil
import sys
from pathlib import Path

import click
from rich.console import Console
from rich.table import Table

from . import __version__
from .config import NodeConfig

log = logging.getLogger(__name__)
err_console = Console(stderr=True)


# --- shared state ---


class CliContext:
    def __init__(
        self,
        config_path: Path | None,
        output: str,
        debug: bool,
    ) -> None:
        self.config_path = config_path
        self.output = output
        self.debug = debug
        self._cfg = None  # type: ignore[assignment]
        self._paths = None  # type: ignore[assignment]

    def load(self) -> tuple:
        if self._cfg is None:
            from .io import load_config

            self._cfg, self._paths = load_config(self.config_path)
        return self._cfg, self._paths


pass_ctx = click.make_pass_decorator(CliContext)


# --- root group ---


@click.group(context_settings={"help_option_names": ["-h", "--help"]})
@click.version_option(__version__, prog_name="nostos")
@click.option(
    "--config",
    "config_path",
    type=click.Path(path_type=Path),
    help="Explicit path to config.yaml (overrides discovery).",
)
@click.option(
    "--output",
    type=click.Choice(["text", "json"]),
    default="text",
    help="Output format.",
)
@click.option("--debug", is_flag=True, help="Show tracebacks on errors.")
@click.pass_context
def cli(ctx: click.Context, config_path: Path | None, output: str, debug: bool) -> None:
    """nostos — bring your bare metal home to the Talos cluster."""
    from .io import configure_logging

    configure_logging(debug)
    ctx.obj = CliContext(config_path=config_path, output=output, debug=debug)


# --- init ---


@cli.command()
@click.option("--force", is_flag=True, help="Overwrite an existing config.yaml.")
@click.argument("directory", type=click.Path(path_type=Path), default=".")
def init(directory: Path, force: bool) -> None:
    """Scaffold config.yaml, templates/, state/ in DIRECTORY."""
    directory = directory.resolve()
    directory.mkdir(parents=True, exist_ok=True)
    cfg = directory / "config.yaml"
    if cfg.exists() and not force:
        err_console.print(f"[red]Error:[/red] {cfg} already exists. Use --force to overwrite.")
        sys.exit(1)
    cfg.write_text(_INIT_CONFIG_TEMPLATE)
    (directory / "templates").mkdir(exist_ok=True)
    state_dir = directory / "state"
    state_dir.mkdir(exist_ok=True)
    (state_dir / ".gitignore").write_text("# everything in here is a cache\n*\n!.gitignore\n")
    Console().print(f"[green]Initialized nostos project at {directory}[/green]")


_INIT_CONFIG_TEMPLATE = """# nostos config.yaml — see .submodules/nostos/README.md for full schema
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  # Get schematic ID from https://factory.talos.dev
  schematic_id: REPLACE-ME

secrets:
  backend: onepassword
  onepassword:
    account: my.1password.com
    vault: kubernetes

nodes: {}
  # dell01:
  #   mac: "d0:94:66:d9:eb:a5"
  #   ip: 192.168.68.100
  #   role: controlplane
  #   arch: amd64
  #   install_disk: /dev/nvme0n1
  #   template: dell01.yaml
"""


# --- node group ---


@cli.group()
def node() -> None:
    """Manage node registrations."""


@node.command("list")
@pass_ctx
def node_list(ctx: CliContext) -> None:
    """List registered nodes with their reachability."""
    from .registry import list_nodes, probe_node

    cfg, _ = ctx.load()
    if ctx.output == "json":
        rows = []
        for name, n in list_nodes(cfg):
            p = probe_node(n, timeout=1.5)
            p.name = name
            rows.append(
                {
                    "name": name,
                    "ip": str(n.ip),
                    "role": n.role,
                    "ping": p.ping,
                    "apid": p.apid,
                    "version": p.version,
                }
            )
        import json as _json

        click.echo(_json.dumps(rows, indent=2))
        return
    table = Table(title=f"Nodes in {cfg.cluster.name}")
    for col in ("name", "ip", "role", "ping", "apid", "version"):
        table.add_column(col)
    for name, n in list_nodes(cfg):
        p = probe_node(n, timeout=1.5)
        table.add_row(
            name,
            str(n.ip),
            n.role,
            _pill(p.ping),
            _pill(p.apid),
            p.version or "-",
        )
    Console().print(table)


@node.command("add")
@click.argument("name")
@pass_ctx
def node_add(ctx: CliContext, name: str) -> None:
    """Interactively add a new node."""
    import questionary

    from .registry import add_node, RegistryError

    cfg, paths = ctx.load()

    mac = questionary.text("MAC address (e.g. d0:94:66:d9:eb:a5):").ask()
    ip = questionary.text("IP address:").ask()
    role = questionary.select("Role:", choices=["controlplane", "worker"]).ask()
    arch = questionary.select("Arch:", choices=["amd64", "arm64"]).ask()
    install_disk = questionary.text("Install disk (e.g. /dev/nvme0n1):").ask()
    template = questionary.text(
        "Template filename under templates/:",
        default=f"{name}.yaml",
    ).ask()

    try:
        new = NodeConfig(
            mac=mac, ip=ip, role=role, arch=arch,
            install_disk=install_disk, template=template,
        )
        add_node(paths.config, name, new)
    except (ValueError, RegistryError) as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)

    tmpl_path = paths.templates / template
    if not tmpl_path.is_file():
        tmpl_path.parent.mkdir(parents=True, exist_ok=True)
        tmpl_path.write_text(
            f"# Talos machineconfig template for {name}\n"
            "# Fill in with op:// refs.\n"
        )
        Console().print(f"[dim]Scaffolded empty template at {tmpl_path}[/dim]")
    Console().print(f"[green]Added {name}[/green]")


@node.command("remove")
@click.argument("name")
@click.option("--yes", is_flag=True, help="Skip confirmation.")
@pass_ctx
def node_remove(ctx: CliContext, name: str, yes: bool) -> None:
    """Remove a node registration."""
    from .registry import remove_node, RegistryError

    cfg, paths = ctx.load()
    if name not in cfg.nodes:
        err_console.print(f"[red]Error:[/red] no such node {name!r}")
        sys.exit(1)
    if not yes:
        err_console.print("Refusing to remove without --yes confirmation.")
        sys.exit(1)
    try:
        remove_node(paths.config, name)
    except RegistryError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)
    Console().print(f"[green]Removed {name}[/green]")


# --- build / render / serve ---


@cli.command()
@click.option("--force", is_flag=True, help="Force rebuild.")
@click.option("--arch", default="amd64")
@pass_ctx
def build(ctx: CliContext, force: bool, arch: str) -> None:
    """Download Talos assets + build iPXE binary."""
    from .pxe.build import BuildError, build_all

    cfg, paths = ctx.load()
    try:
        build_all(cfg, paths, force=force, arch=arch)
    except BuildError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)
    Console().print(f"[green]Assets ready in {paths.assets}[/green]")


@cli.command()
@click.argument("node_name")
@click.option("--no-validate", is_flag=True, help="Skip talosctl validate.")
@pass_ctx
def render(ctx: CliContext, node_name: str, no_validate: bool) -> None:
    """Render a machineconfig for NODE_NAME with secrets injected."""
    from .registry import render_node, RegistryError
    from .secrets.base import SecretResolutionError

    cfg, paths = ctx.load()
    try:
        out = render_node(cfg, paths, node_name, run_validate=not no_validate)
    except RegistryError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)
    except SecretResolutionError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)
    Console().print(f"[green]Rendered {out}[/green]")


@cli.command()
@click.option("--iface", default=None, help="Network interface (auto-detect if unset).")
@click.option("--port", default=9080, type=int, help="HTTP port.")
@click.option("--down", is_flag=True, help="Kill any stale nostos serve processes and exit.")
@pass_ctx
def serve(ctx: CliContext, iface: str | None, port: int, down: bool) -> None:
    """Start the PXE server (HTTP + dnsmasq). Ctrl+C to stop."""
    from .pxe.serve import Server, ServeError, tear_down_stale

    if down:
        tear_down_stale(http_port=port)
        Console().print("[green]Stopped any stale nostos serve[/green]")
        return

    _, paths = ctx.load()
    try:
        Server(paths, http_port=port, iface=iface).run()
    except ServeError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)


# --- install cheat-sheet ---


@cli.command()
@click.argument("node_name")
@pass_ctx
def install(ctx: CliContext, node_name: str) -> None:
    """Print a per-node boot cheat-sheet (BIOS keys, PXE flow, expected states)."""
    from .registry import get_node

    cfg, _ = ctx.load()
    n = get_node(cfg, node_name)
    Console().print(_install_cheatsheet(node_name, n))


def _install_cheatsheet(name: str, n: NodeConfig) -> str:
    return f"""[bold cyan]Installing {name}[/bold cyan]
  MAC:           {n.mac}
  Target IP:     {n.ip}
  Install disk:  {n.install_disk}
  Arch:          {n.arch}
  Role:          {n.role}

[bold]Boot sequence:[/bold]
  1. Power on (or reboot) the node.
  2. At POST, tap [yellow]F12[/yellow] for the one-time boot menu
     (or [yellow]F2[/yellow] for BIOS setup to make PXE the default).
  3. Select [green]UEFI: Onboard NIC IPv4[/green] (or similar).
  4. Watch the nostos serve terminal for GET requests:
       GET /boot.ipxe
       GET /vmlinuz-{n.arch}
       GET /initramfs-{n.arch}.xz
       GET /configs/{n.mac_hyphen}.yaml  ← confirms config was fetched
  5. Talos installs to {n.install_disk} and reboots.
  6. After reboot, the node boots from disk with persisted config.
     Ensure BIOS boot order has HDD/NVMe before PXE to avoid loops.

[bold]After install:[/bold]
  nostos bootstrap {name}       (first controlplane only)
  nostos status
"""


# --- wipe / bootstrap / config refresh / status / kubeconfig / nuke ---


@cli.command()
@click.argument("node_name")
@pass_ctx
def wipe(ctx: CliContext, node_name: str) -> None:
    """Mark NODE_NAME for a one-shot disk wipe on next PXE boot."""
    from .cluster.wipe import queue_wipe
    from .registry import get_node

    cfg, paths = ctx.load()
    n = get_node(cfg, node_name)
    queue_wipe(paths, n.mac)
    Console().print(f"[yellow]Queued wipe for {node_name} ({n.mac})[/yellow]")
    Console().print("Reboot the node into PXE; the next install will wipe the system disk.")


@cli.command()
@click.argument("node_name")
@click.option("--timeout", default=300.0, type=float)
@pass_ctx
def bootstrap(ctx: CliContext, node_name: str, timeout: float) -> None:
    """Bootstrap etcd on NODE_NAME (first controlplane only)."""
    from .cluster.bootstrap import BootstrapError, bootstrap_node, fetch_kubeconfig
    from .registry import get_node

    cfg, paths = ctx.load()
    n = get_node(cfg, node_name)
    try:
        bootstrap_node(cfg, paths, n, timeout=timeout)
        fetch_kubeconfig(paths, n)
    except BootstrapError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)
    Console().print(f"[green]Bootstrapped {node_name}. kubeconfig: {paths.kubeconfig}[/green]")


@cli.group(name="config")
def config_group() -> None:
    """Config subcommands."""


@config_group.command("refresh")
@click.option(
    "--hours", default=876_000, type=int, help="Admin cert validity in hours (default ~100y)."
)
@click.option(
    "--controlplane",
    default=None,
    help="Controlplane node name to extract CA from. Default: first controlplane in config.",
)
@pass_ctx
def config_refresh(ctx: CliContext, hours: int, controlplane: str | None) -> None:
    """Regenerate admin client certificate against the existing CA."""
    from .cluster.cert import CertRefreshError, refresh_admin_cert

    cfg, paths = ctx.load()
    if controlplane is None:
        controlplane = next(
            (name for name, n in cfg.nodes.items() if n.role == "controlplane"),
            None,
        )
        if controlplane is None:
            err_console.print("[red]Error:[/red] no controlplane node in config.yaml")
            sys.exit(1)
    node = cfg.nodes[controlplane]
    try:
        refresh_admin_cert(cfg, paths, node, hours=hours)
    except CertRefreshError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)
    Console().print(f"[green]Wrote {paths.talosconfig} (valid {hours}h)[/green]")


@cli.command()
@pass_ctx
def status(ctx: CliContext) -> None:
    """Show per-node reachability and Talos version."""
    from .cluster.status import cluster_status

    cfg, _ = ctx.load()
    st = cluster_status(cfg, timeout=1.5)
    if ctx.output == "json":
        import json as _json

        click.echo(
            _json.dumps(
                [
                    {
                        "name": n.name,
                        "ip": n.ip,
                        "role": n.role,
                        "ping": n.ping,
                        "apid": n.apid,
                        "version": n.version,
                    }
                    for n in st.nodes
                ],
                indent=2,
            )
        )
        return
    table = Table(title=f"Cluster {cfg.cluster.name}")
    for col in ("name", "ip", "role", "ping", "apid", "version"):
        table.add_column(col)
    for n in st.nodes:
        table.add_row(n.name, n.ip, n.role, _pill(n.ping), _pill(n.apid), n.version or "-")
    Console().print(table)


@cli.command()
@click.argument("node_name", required=False)
@pass_ctx
def kubeconfig(ctx: CliContext, node_name: str | None) -> None:
    """Refresh the cluster kubeconfig at state/kubeconfig."""
    from .cluster.bootstrap import BootstrapError, fetch_kubeconfig
    from .registry import get_node

    cfg, paths = ctx.load()
    if node_name is None:
        node_name = next(
            (name for name, n in cfg.nodes.items() if n.role == "controlplane"),
            None,
        )
        if node_name is None:
            err_console.print("[red]Error:[/red] no controlplane node in config")
            sys.exit(1)
    n = get_node(cfg, node_name)
    try:
        fetch_kubeconfig(paths, n)
    except BootstrapError as e:
        err_console.print(f"[red]Error:[/red] {e}")
        sys.exit(1)
    Console().print(f"[green]kubeconfig written to {paths.kubeconfig}[/green]")


@cli.command()
@click.option("--yes", is_flag=True, help="Skip confirmation.")
@pass_ctx
def nuke(ctx: CliContext, yes: bool) -> None:
    """Delete the entire state/ cache. Safe: can be rebuilt from config + secrets."""
    _, paths = ctx.load()
    if not paths.state.is_dir():
        Console().print("Nothing to nuke.")
        return
    if not yes:
        confirm = click.confirm(f"Remove {paths.state}?", default=False)
        if not confirm:
            return
    shutil.rmtree(paths.state)
    Console().print(f"[green]Removed {paths.state}[/green]")


# --- web ---


@cli.command()
@click.option("--host", default="127.0.0.1")
@click.option("--port", default=8080, type=int)
@click.option("--read-only", is_flag=True, help="Disable mutation endpoints.")
@click.option(
    "--i-know-what-im-doing",
    "override_host_safety",
    is_flag=True,
    help="Required to bind to non-loopback.",
)
@pass_ctx
def web(
    ctx: CliContext,
    host: str,
    port: int,
    read_only: bool,
    override_host_safety: bool,
) -> None:
    """Start the local web dashboard (optional)."""
    if host not in ("127.0.0.1", "localhost", "::1") and not override_host_safety:
        err_console.print(
            f"[red]Refusing to bind to {host}[/red] without --i-know-what-im-doing. "
            "nostos web has no auth; loopback only."
        )
        sys.exit(1)
    try:
        from .web.app import run_server
    except ImportError:
        err_console.print(
            "[red]Error:[/red] web extras not installed. "
            "Run: uv tool install --editable .[web]"
        )
        sys.exit(1)
    cfg, paths = ctx.load()
    run_server(cfg, paths, host=host, port=port, read_only=read_only)


# --- helpers ---


def _pill(state: str) -> str:
    colors = {
        "up": "green",
        "down": "red",
        "refused": "yellow",
        "unknown": "dim",
    }
    c = colors.get(state, "white")
    return f"[{c}]{state}[/{c}]"


def main() -> None:  # pragma: no cover
    try:
        cli()
    except Exception as e:
        # Unhandled — show a minimal message unless --debug (click handles --debug
        # by virtue of the fact that the exception will propagate if click didn't
        # consume it).
        err_console.print(f"[red]Unexpected error:[/red] {e}")
        sys.exit(2)


if __name__ == "__main__":  # pragma: no cover
    main()
