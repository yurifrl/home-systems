#!/usr/bin/env bash

# Check if running on nixos-1
if [[ $(hostname) != "nixos-1" ]]; then
    echo "Error: This script must be run on nixos-1"
    exit 1
fi

# Check for required argument
if [ $# -ne 1 ]; then
    echo "Usage: $0 [on|off|status]"
    exit 1
fi

# Define directories to manage
DIRS=(
    "/var/lib/rook"
)

cleanup() {
    # First find and unmount any tmpfs mounts under our managed paths
    for dir in "${DIRS[@]}"; do
        # Find all tmpfs mounts under this directory and unmount them (in reverse order)
        findmnt -t tmpfs -n | grep "^/tmp$dir" | tac | cut -d' ' -f1 | while read -r mount_point; do
            sudo umount "$mount_point" 2>/dev/null || true
        done
        # Unmount the main directory if it's mounted
        sudo umount "/tmp$dir" 2>/dev/null || true
    done

    # Now safe to remove directories
    for dir in "${DIRS[@]}"; do
        sudo rm -rf "$dir"
        sudo rm -rf "/tmp$dir"
    done
}

create_temp() {
    # Create temporary directories with tmpfs mounts
    for dir in "${DIRS[@]}"; do
        sudo mkdir -p "/tmp$dir"
        sudo mount -t tmpfs tmpfs "/tmp$dir"
    done
}

check_status() {
    echo "Checking mount status for managed directories:"
    for dir in "${DIRS[@]}"; do
        if [[ -L "$dir" ]]; then
            echo "$dir -> $(readlink "$dir") (symlink)"
            if findmnt "/tmp$dir" | grep -q tmpfs; then
                echo "  ├── tmpfs mounted"
            else
                echo "  ├── NO tmpfs mount"
            fi
        else
            echo "$dir (regular directory)"
        fi
    done
}

case "$1" in
    "on")
        cleanup
        create_temp
        # Create symlinks
        for dir in "${DIRS[@]}"; do
            sudo ln -sf "/tmp$dir" "$dir"
        done
        echo "Ephemeral disks enabled"
        ;;
    "off")
        # Unmount tmpfs before cleanup
        for dir in "${DIRS[@]}"; do
            sudo umount "/tmp$dir" 2>/dev/null || true
        done
        cleanup
        echo "Ephemeral disks disabled"
        ;;
    "status")
        check_status
        ;;
    *)
        echo "Usage: $0 [on|off|status]"
        exit 1
        ;;
esac