#!/usr/bin/env bash

# Get the disk's identifier
DISK_ID=$(sudo udevadm info --query=property --name=/dev/sda | grep ID_SERIAL_SHORT= | cut -d= -f2)
MODEL=$(sudo udevadm info --query=property --name=/dev/sda | grep ID_MODEL= | cut -d= -f2)
LAYOUT_FILE="disks/${MODEL}_${DISK_ID}.layout"

# Check if layout file exists
if [ ! -f "$LAYOUT_FILE" ]; then
    echo "No layout file found for this disk: $LAYOUT_FILE"
    echo "Current disk ID: ${MODEL}_${DISK_ID}"
    exit 1
fi

# Proceed with partitioning
echo "Found layout file: $LAYOUT_FILE"
echo "Proceeding with partitioning..."
sudo sfdisk /dev/sda --wipe=always < "$LAYOUT_FILE"
sudo mkfs.ext4 /dev/sda1