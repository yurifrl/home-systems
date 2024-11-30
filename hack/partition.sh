#!/usr/bin/env bash

# List all available disks
echo "=== Available Disks ==="
lsblk -d -o NAME,SIZE,MODEL,SERIAL | grep -v "loop"
echo ""

# Prompt for disk selection
read -p "Enter disk name to partition (e.g., sda): " DISK_NAME
if [ -z "$DISK_NAME" ]; then
    echo "No disk selected. Exiting."
    exit 1
fi

# Get the disk's identifier
DISK_ID=$(sudo udevadm info --query=property --name=/dev/${DISK_NAME} | grep ID_SERIAL_SHORT= | cut -d= -f2)
MODEL=$(sudo udevadm info --query=property --name=/dev/${DISK_NAME} | grep ID_MODEL= | cut -d= -f2)
LAYOUT_FILE="disks/${MODEL}_${DISK_ID}.layout"

# Check if layout file exists
if [ ! -f "$LAYOUT_FILE" ]; then
    echo "No layout file found for this disk: $LAYOUT_FILE"
    echo "Current disk ID: ${MODEL}_${DISK_ID}"
    exit 1
fi

# Display disk information
echo "=== Disk Information ==="
echo "Device: /dev/${DISK_NAME}"
echo "Model: $MODEL"
echo "Serial: $DISK_ID"
echo "Layout file: $LAYOUT_FILE"
echo ""
echo "Current partition layout:"
sudo fdisk -l /dev/${DISK_NAME}
echo ""
echo "WARNING: This will ERASE ALL DATA on /dev/${DISK_NAME}"
echo "Layout to be applied:"
cat "$LAYOUT_FILE"
echo ""

# Prompt for confirmation
read -p "Are you sure you want to proceed? (yes/no) " answer
if [ "$answer" != "yes" ]; then
    echo "Aborted."
    exit 1
fi

# Proceed with partitioning
echo "Proceeding with partitioning..."
sudo sfdisk /dev/${DISK_NAME} --wipe=always < "$LAYOUT_FILE"
sudo mkfs.ext4 /dev/${DISK_NAME}1