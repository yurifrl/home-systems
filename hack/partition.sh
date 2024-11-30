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

# Get the full disk info
DISK_INFO=$(sudo udevadm info --query=property --name=/dev/${DISK_NAME})

# Look for existing layout file that matches this disk
LAYOUT_FILE=$(find disks -name "*.sfdisk" -type f | grep -F "$(echo "$DISK_INFO" | grep ID_SERIAL= | cut -d= -f2)")

if [ ! -f "$LAYOUT_FILE" ]; then
    echo "No layout file found for this disk"
    echo "Current disk info:"
    echo "$DISK_INFO" | grep -E "ID_SERIAL=|ID_MODEL="
    exit 1
fi

# Display disk information
echo "================================================================"
echo "                     === Disk Information ===                     "
echo "================================================================"
echo "Device: /dev/${DISK_NAME}"
echo "Model: $(echo "$DISK_INFO" | grep ID_MODEL= | cut -d= -f2)"
echo "Serial: $(echo "$DISK_INFO" | grep ID_SERIAL= | cut -d= -f2)"
echo "Layout file: $LAYOUT_FILE"
echo "----------------------------------------------------------------"
echo ""

echo "Current partition layout:"
echo "----------------------------------------------------------------"
sudo fdisk -l /dev/${DISK_NAME}
echo "----------------------------------------------------------------"
echo ""

echo "================================================================"
echo "WARNING: This will ERASE ALL DATA on /dev/${DISK_NAME}"
echo "================================================================"
echo "Layout to be applied:"
echo "----------------------------------------------------------------"
cat "$LAYOUT_FILE"
echo "----------------------------------------------------------------"
echo ""

# Prompt for confirmation
read -p "Are you sure you want to proceed? (yes/no) " answer
if [ "$answer" != "yes" ]; then
    echo "Aborted."
    exit 1
fi

# Proceed with partitioning
echo "================================================================"
echo "                === Starting Partitioning Process ===            "
echo "================================================================"
echo "Wiping existing filesystem signatures..."
sudo wipefs -a /dev/${DISK_NAME}

echo "----------------------------------------------------------------"
echo "Applying partition layout..."
sudo sfdisk --force /dev/${DISK_NAME} < "$LAYOUT_FILE"

# Wait a moment for the kernel to register the new partitions
sleep 2

echo "----------------------------------------------------------------"
echo "Formatting /dev/${DISK_NAME}1..."
sudo mkfs.ext4 -F /dev/${DISK_NAME}1
echo "================================================================"