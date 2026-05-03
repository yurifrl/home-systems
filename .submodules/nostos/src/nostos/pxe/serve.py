"""PXE serving — orchestrates python HTTP + dnsmasq subprocesses.

Designed for macOS + Linux operator hosts. dnsmasq requires sudo to bind
DHCP port 67. HTTP is user-level on port 9080.
"""

from __future__ import annotations

import atexit
import logging
import os
import shutil
import signal
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path

from ..paths import Paths

log = logging.getLogger(__name__)

DEFAULT_HTTP_PORT = 9080
DEFAULT_DHCP_RANGE_START = "192.168.68.200"
DEFAULT_DHCP_RANGE_END = "192.168.68.210"
DEFAULT_TFTP_ROOT = "/tmp/nostos-tftp"


class ServeError(RuntimeError):
    """Raised on serve-time setup problems."""


@dataclass
class NetworkInfo:
    """Detected host networking."""

    interface: str
    ip: str


def detect_network(subnet_hint: str = "192.168.68.") -> NetworkInfo:
    """Find the ethernet interface carrying the target subnet.

    Replaces the old ``detect-mac-ip.sh``. Skips Wi-Fi when possible —
    Wi-Fi loses the DHCP race against most consumer routers.
    """
    import ipaddress

    try:
        # macOS + Linux both have `ifconfig` or `ip -o addr` — use stdlib socket
        import psutil  # type: ignore[import-not-found]
    except ImportError:
        return _detect_via_ifconfig(subnet_hint)

    # Prefer psutil when available (more reliable).
    addrs = psutil.net_if_addrs()  # type: ignore[attr-defined]
    for iface, family_list in addrs.items():
        if iface.startswith(("lo", "utun", "awdl", "llw", "bridge", "stf", "gif", "anpi", "ap")):
            continue
        for entry in family_list:
            if entry.family == 2:  # AF_INET
                try:
                    ip_obj = ipaddress.ip_address(entry.address)
                except ValueError:
                    continue
                if str(ip_obj).startswith(subnet_hint):
                    return NetworkInfo(interface=iface, ip=str(ip_obj))
    raise ServeError(
        f"no interface found carrying {subnet_hint}x/24. "
        "Connect the operator host to the same switch as the target node."
    )


def _detect_via_ifconfig(subnet_hint: str) -> NetworkInfo:
    """Fallback: parse `ifconfig` output."""
    import re

    try:
        result = subprocess.run(
            ["ifconfig"], check=True, capture_output=True, text=True, timeout=5
        )
    except (subprocess.CalledProcessError, FileNotFoundError) as e:
        raise ServeError("cannot run ifconfig; install psutil or pass --iface") from e
    current_iface = None
    for line in result.stdout.splitlines():
        m = re.match(r"^(\w+):", line)
        if m:
            current_iface = m.group(1)
            continue
        if current_iface and subnet_hint in line:
            m2 = re.search(rf"inet ({re.escape(subnet_hint)}\d+)", line)
            if m2:
                if current_iface.startswith(("lo", "utun", "awdl", "llw", "bridge", "stf", "gif", "anpi", "ap")):
                    continue
                return NetworkInfo(interface=current_iface, ip=m2.group(1))
    raise ServeError(f"no interface found carrying {subnet_hint}x/24")


class Server:
    """Run HTTP + dnsmasq subprocesses in the foreground until interrupted."""

    def __init__(
        self,
        paths: Paths,
        *,
        http_port: int = DEFAULT_HTTP_PORT,
        dhcp_range_start: str = DEFAULT_DHCP_RANGE_START,
        dhcp_range_end: str = DEFAULT_DHCP_RANGE_END,
        gateway: str = "192.168.68.1",
        tftp_root: str = DEFAULT_TFTP_ROOT,
        iface: str | None = None,
    ) -> None:
        self.paths = paths
        self.http_port = http_port
        self.dhcp_range = (dhcp_range_start, dhcp_range_end)
        self.gateway = gateway
        self.tftp_root = Path(tftp_root)
        self.iface = iface  # None → auto-detect
        self.http_proc: subprocess.Popen[bytes] | None = None
        self.dnsmasq_proc: subprocess.Popen[bytes] | None = None

    def preflight(self) -> NetworkInfo:
        """Verify assets exist, tools are available, detect interface."""
        for required in ("ipxe.efi", "boot.ipxe"):
            if not (self.paths.assets / required).is_file():
                raise ServeError(
                    f"missing {required} under {self.paths.assets}. Run `nostos build` first."
                )
        if shutil.which("dnsmasq") is None and not Path("/opt/homebrew/sbin/dnsmasq").is_file():
            raise ServeError(
                "dnsmasq not found. Install via: brew install dnsmasq"
            )
        net = detect_network() if self.iface is None else NetworkInfo(interface=self.iface, ip="unknown")
        log.info("serving on %s (%s)", net.interface, net.ip)
        return net

    def stage_tftp(self) -> None:
        """Copy ipxe.efi to a world-readable path dnsmasq/nobody can read."""
        self.tftp_root.mkdir(parents=True, exist_ok=True)
        src = self.paths.assets / "ipxe.efi"
        dst = self.tftp_root / "ipxe.efi"
        shutil.copyfile(src, dst)
        os.chmod(self.tftp_root, 0o755)
        os.chmod(dst, 0o644)

    def kill_stale_http(self) -> None:
        """Kill any stale process bound to the configured HTTP port."""
        try:
            out = subprocess.run(
                ["lsof", "-ti", f":{self.http_port}"],
                capture_output=True,
                text=True,
                timeout=5,
            )
        except (subprocess.TimeoutExpired, FileNotFoundError):
            return
        for pid_str in out.stdout.strip().splitlines():
            try:
                pid = int(pid_str)
            except ValueError:
                continue
            log.info("killing stale process on :%s (pid %s)", self.http_port, pid)
            try:
                os.kill(pid, signal.SIGTERM)
            except OSError:
                pass
        if out.stdout.strip():
            time.sleep(1)

    def start_http(self) -> None:
        self.http_proc = subprocess.Popen(
            [sys.executable, "-m", "http.server", str(self.http_port)],
            cwd=str(self.paths.assets),
        )
        log.info("HTTP server PID %s on :%s", self.http_proc.pid, self.http_port)

    def start_dnsmasq(self, net: NetworkInfo) -> None:
        dnsmasq_bin = "/opt/homebrew/sbin/dnsmasq"
        if not Path(dnsmasq_bin).is_file():
            dnsmasq_bin = shutil.which("dnsmasq") or "dnsmasq"

        args: list[str] = [
            "sudo",
            dnsmasq_bin,
            "--no-daemon",
            "--port=0",
            f"--interface={net.interface}",
            f"--dhcp-range={self.dhcp_range[0]},{self.dhcp_range[1]},255.255.255.0,5m",
            f"--dhcp-option=3,{self.gateway}",
            f"--dhcp-option=6,{self.gateway}",
            "--dhcp-authoritative",
            "--dhcp-match=set:pxe,60,PXEClient",
            "--dhcp-ignore=tag:!pxe",
            "--enable-tftp",
            f"--tftp-root={self.tftp_root}",
            "--dhcp-userclass=set:ipxe,iPXE",
            f"--dhcp-boot=tag:!ipxe,ipxe.efi,,{net.ip}",
            f"--dhcp-boot=tag:ipxe,http://{net.ip}:{self.http_port}/boot.ipxe",
            "--log-queries",
            "--log-dhcp",
        ]
        self.dnsmasq_proc = subprocess.Popen(args)

    def stop(self) -> None:
        """Terminate children on shutdown."""
        for proc, name in [
            (self.dnsmasq_proc, "dnsmasq"),
            (self.http_proc, "http"),
        ]:
            if proc is None:
                continue
            if proc.poll() is None:
                log.info("stopping %s (pid %s)", name, proc.pid)
                try:
                    proc.terminate()
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
                except OSError:
                    pass

    def run(self) -> int:
        """Foreground runloop. Returns dnsmasq's exit code."""
        net = self.preflight()
        self.kill_stale_http()
        self.stage_tftp()
        self.start_http()

        atexit.register(self.stop)

        def _sig_handler(signum: int, frame) -> None:
            self.stop()
            sys.exit(128 + signum)

        signal.signal(signal.SIGINT, _sig_handler)
        signal.signal(signal.SIGTERM, _sig_handler)

        self.start_dnsmasq(net)
        assert self.dnsmasq_proc is not None
        try:
            return self.dnsmasq_proc.wait()
        finally:
            self.stop()


def tear_down_stale(http_port: int = DEFAULT_HTTP_PORT) -> None:
    """`nostos serve --down` implementation."""
    try:
        subprocess.run(
            ["pkill", "-f", "python3 -m http.server " + str(http_port)],
            check=False,
            timeout=5,
        )
        subprocess.run(
            ["sudo", "-n", "pkill", "-f", "dnsmasq.*tftp-root"],
            check=False,
            timeout=5,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError):
        pass
