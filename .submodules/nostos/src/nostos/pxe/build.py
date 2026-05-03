"""Build PXE boot assets — kernel, initramfs, iPXE binary, boot.ipxe script.

Downloads Talos kernel + initramfs from the Image Factory. Cross-compiles
iPXE via a Docker container with an embedded retry-loop chainload script
that tolerates consumer-router DHCP races.
"""

from __future__ import annotations

import logging
import shutil
import subprocess
from pathlib import Path

import httpx

from ..config import NostosConfig
from ..paths import Paths

log = logging.getLogger(__name__)

IPXE_REPO = "https://github.com/ipxe/ipxe.git"
IPXE_MAX_SIZE_BYTES = 256 * 1024  # Dell UEFI TFTP buffer limit

EMBED_IPXE = """#!ipxe
# Keep DHCP'ing until the bootfile-name from OUR dnsmasq wins.
# Home-lab routers (e.g. TP-Link Deco) also reply to DHCP but don't set
# option 67 (bootfile-name / ${filename}), so we retry until it's set.
:retry_dhcp
dhcp || goto retry_dhcp
isset ${filename} || goto retry_dhcp
chain ${filename}
"""

BOOT_IPXE_TEMPLATE = """#!ipxe
# Rendered by nostos. Per-MAC config fetched via ${{mac:hexhyp}}.
# ${{next-server}} is resolved by iPXE at runtime from the DHCP reply,
# so this script is IP-portable (no rebuild when the operator's IP changes).
#
# To force a one-shot disk wipe during reinstall, pending_wipes logic in
# nostos serve adds talos.experimental.wipe=system to the kernel cmdline.
set cfg_url http://${{next-server}}:9080/configs/${{mac:hexhyp}}.yaml
echo Booting Talos {talos_version}, config at ${{cfg_url}}
kernel http://${{next-server}}:9080/vmlinuz-{arch} talos.platform=metal talos.config=${{cfg_url}} {extra_args} slab_nomerge pti=on console=tty0
initrd http://${{next-server}}:9080/initramfs-{arch}.xz
boot
"""


class BuildError(RuntimeError):
    """Raised when asset build fails in a way the user must see."""


def build_all(cfg: NostosConfig, paths: Paths, *, force: bool = False, arch: str = "amd64") -> None:
    """Download Talos assets, build iPXE, render boot.ipxe."""
    paths.ensure_state()
    download_talos_assets(cfg, paths, arch=arch, force=force)
    build_ipxe(paths, force=force)
    render_boot_ipxe(cfg, paths, arch=arch)


def download_talos_assets(
    cfg: NostosConfig, paths: Paths, *, arch: str = "amd64", force: bool = False
) -> None:
    """Fetch vmlinuz + initramfs.xz from factory.talos.dev."""
    base = f"https://factory.talos.dev/image/{cfg.cluster.schematic_id}/{cfg.cluster.talos_version}"
    kernel_url = f"{base}/kernel-{arch}"
    initramfs_url = f"{base}/initramfs-{arch}.xz"
    kernel_path = paths.assets / f"vmlinuz-{arch}"
    initramfs_path = paths.assets / f"initramfs-{arch}.xz"
    _download(kernel_url, kernel_path, force=force)
    _download(initramfs_url, initramfs_path, force=force)


def build_ipxe(paths: Paths, *, force: bool = False) -> None:
    """Clone iPXE and cross-compile snponly.efi inside a Docker container."""
    ipxe_efi = paths.assets / "ipxe.efi"
    embed_path = paths.ipxe_src / "src" / "embed.ipxe"

    # Rebuild if: force, or binary missing, or embed script out of date.
    if not force and ipxe_efi.is_file() and embed_path.is_file() and embed_path.read_text() == EMBED_IPXE:
        log.info("iPXE up to date at %s", ipxe_efi)
        return

    if shutil.which("docker") is None:
        raise BuildError(
            "Docker is required to build iPXE in v0.1. "
            "Install Docker Desktop or OrbStack, or wait for v0.2 pre-built binaries."
        )

    _clone_ipxe(paths)
    embed_path.parent.mkdir(parents=True, exist_ok=True)
    embed_path.write_text(EMBED_IPXE)

    log.info("building iPXE snponly.efi via Docker")
    try:
        subprocess.run(
            [
                "docker",
                "run",
                "--rm",
                "--platform",
                "linux/amd64",
                "-v",
                f"{paths.ipxe_src}:/ipxe",
                "-w",
                "/ipxe/src",
                "alpine:latest",
                "sh",
                "-c",
                "apk add --no-cache --quiet build-base perl xz-dev mtools gnu-efi-dev >/dev/null && "
                "make -j4 bin-x86_64-efi/snponly.efi EMBED=embed.ipxe NO_WERROR=1",
            ],
            check=True,
            capture_output=True,
            text=True,
            timeout=600,
        )
    except subprocess.CalledProcessError as e:
        raise BuildError(
            f"iPXE build failed (exit {e.returncode}): {e.stderr[-500:] if e.stderr else ''}"
        ) from e
    except subprocess.TimeoutExpired as e:
        raise BuildError("iPXE build timed out after 10 minutes") from e

    built = paths.ipxe_src / "src" / "bin-x86_64-efi" / "snponly.efi"
    if not built.is_file():
        raise BuildError(f"iPXE build did not produce {built}")
    shutil.copyfile(built, ipxe_efi)

    size = ipxe_efi.stat().st_size
    if size > IPXE_MAX_SIZE_BYTES:
        raise BuildError(
            f"iPXE binary is {size} bytes, exceeds Dell UEFI TFTP limit of {IPXE_MAX_SIZE_BYTES}"
        )
    log.info("iPXE built: %s (%d bytes)", ipxe_efi, size)


def render_boot_ipxe(
    cfg: NostosConfig, paths: Paths, *, arch: str = "amd64", extra_kernel_args: str = ""
) -> Path:
    """Write `state/assets/boot.ipxe` with ${next-server} templating."""
    out = paths.assets / "boot.ipxe"
    out.parent.mkdir(parents=True, exist_ok=True)
    content = BOOT_IPXE_TEMPLATE.format(
        talos_version=cfg.cluster.talos_version,
        arch=arch,
        extra_args=extra_kernel_args,
    )
    out.write_text(content)
    return out


# --- internals ---


def _download(url: str, dest: Path, *, force: bool) -> None:
    if dest.is_file() and not force:
        log.info("asset up to date: %s", dest.name)
        return
    log.info("downloading %s", url)
    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_suffix(dest.suffix + ".partial")
    try:
        with httpx.stream("GET", url, timeout=60.0, follow_redirects=True) as r:
            r.raise_for_status()
            with tmp.open("wb") as fp:
                for chunk in r.iter_bytes(1024 * 64):
                    fp.write(chunk)
    except httpx.HTTPError as e:
        tmp.unlink(missing_ok=True)
        raise BuildError(f"download {url} failed: {e}") from e
    tmp.replace(dest)


def _clone_ipxe(paths: Paths) -> None:
    if (paths.ipxe_src / ".git").is_dir():
        return
    if shutil.which("git") is None:
        raise BuildError("git is required to clone iPXE source")
    log.info("cloning iPXE to %s", paths.ipxe_src)
    try:
        subprocess.run(
            ["git", "clone", "--depth", "1", IPXE_REPO, str(paths.ipxe_src)],
            check=True,
            capture_output=True,
            text=True,
            timeout=120,
        )
    except subprocess.CalledProcessError as e:
        raise BuildError(f"iPXE clone failed: {e.stderr.strip()}") from e
