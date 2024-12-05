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
    "/etc/rancher/k3s"
    "/var/lib/rancher/k3s"
    "/var/lib/rook"
)
FILES=(
    "/var/log/k3s.log"
)

cleanup() {
    # Remove all directories
    for dir in "${DIRS[@]}"; do
        sudo rm -rf "$dir"
        sudo rm -rf "/tmp$dir"
    done

    # Remove all files
    for file in "${FILES[@]}"; do
        sudo rm -f "$file"
        sudo rm -f "/tmp$file"
    done
}

create_temp() {
    # Create temporary directories with tmpfs mounts
    for dir in "${DIRS[@]}"; do
        sudo mkdir -p "/tmp$dir"
        sudo mount -t tmpfs tmpfs "/tmp$dir"
    done

    # Create temporary files
    for file in "${FILES[@]}"; do
        sudo mkdir -p "$(dirname "/tmp$file")"
        sudo touch "/tmp$file"
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

    echo -e "\nChecking mount status for managed files:"
    for file in "${FILES[@]}"; do
        if [[ -L "$file" ]]; then
            echo "$file -> $(readlink "$file") (symlink)"
        else
            echo "$file (regular file)"
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
        for file in "${FILES[@]}"; do
            sudo ln -sf "/tmp$file" "$file"
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