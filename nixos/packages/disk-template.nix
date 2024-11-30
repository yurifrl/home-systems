{ stdenv, bash, util-linux, e2fsprogs, udev, findutils, grep, coreutils, writeShellScriptBin }:

let
  # Define disk layouts
  diskLayouts = {
    "USB_SanDisk_3.2Gen1_04016ae56ccfac4811a0010dddda218064618aee1065dfdc49eb60ee9f9161eba17a00000000000000000000ef6dbef8ff077b1867558107ab286235-0:0" = ''
      label: gpt
      unit: sectors

      /dev/sda1 : start=2048, size=2097152, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4
      /dev/sda2 : type=0FC63DAF-8483-4772-8E79-3D69D8477DE4
    '';
  };

  # Create a temporary directory with the layout files
  createLayoutFiles = ''
    mkdir -p disks
    ${builtins.concatStringsSep "\n" (builtins.mapAttrs (name: content: ''
      echo '${content}' > disks/${name}.sfdisk
    '') diskLayouts)}
  '';

  script = ''
    #!/usr/bin/env bash

    # Create layout files in current directory
    ${createLayoutFiles}

    # List all available disks
    echo "=== Available Disks ==="
    ${util-linux}/bin/lsblk -d -o NAME,SIZE,MODEL,SERIAL | ${grep}/bin/grep -v "loop"
    echo ""

    # Prompt for disk selection
    read -p "Enter disk name to partition (e.g., sda): " DISK_NAME
    if [ -z "$DISK_NAME" ]; then
        echo "No disk selected. Exiting."
        exit 1
    fi

    # Get the full disk info
    DISK_INFO=$(${udev}/bin/udevadm info --query=property --name=/dev/''${DISK_NAME})

    # Look for existing layout file that matches this disk
    LAYOUT_FILE=$(${findutils}/bin/find disks -name "*.sfdisk" -type f | ${grep}/bin/grep -F "$(echo "$DISK_INFO" | ${grep}/bin/grep ID_SERIAL= | cut -d= -f2)")

    if [ ! -f "$LAYOUT_FILE" ]; then
        echo "No layout file found for this disk"
        echo "Current disk info:"
        echo "$DISK_INFO" | ${grep}/bin/grep -E "ID_SERIAL=|ID_MODEL="
        exit 1
    fi

    # Display disk information
    echo "================================================================"
    echo "                     === Disk Information ===                     "
    echo "================================================================"
    echo "Device: /dev/''${DISK_NAME}"
    echo "Model: $(echo "$DISK_INFO" | ${grep}/bin/grep ID_MODEL= | cut -d= -f2)"
    echo "Serial: $(echo "$DISK_INFO" | ${grep}/bin/grep ID_SERIAL= | cut -d= -f2)"
    echo "Layout file: $LAYOUT_FILE"
    echo "----------------------------------------------------------------"
    echo ""

    echo "Current partition layout:"
    echo "----------------------------------------------------------------"
    ${util-linux}/bin/fdisk -l /dev/''${DISK_NAME}
    echo "----------------------------------------------------------------"
    echo ""

    echo "================================================================"
    echo "WARNING: This will ERASE ALL DATA on /dev/''${DISK_NAME}"
    echo "================================================================"
    echo "Layout to be applied:"
    echo "----------------------------------------------------------------"
    ${coreutils}/bin/cat "$LAYOUT_FILE"
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
    ${util-linux}/bin/wipefs -a /dev/''${DISK_NAME}

    echo "----------------------------------------------------------------"
    echo "Applying partition layout..."
    ${util-linux}/bin/sfdisk --force /dev/''${DISK_NAME} < "$LAYOUT_FILE"

    # Wait a moment for the kernel to register the new partitions
    ${coreutils}/bin/sleep 2

    echo "----------------------------------------------------------------"
    echo "Formatting /dev/''${DISK_NAME}1..."
    ${e2fsprogs}/bin/mkfs.ext4 -F /dev/''${DISK_NAME}1
    echo "================================================================"
  '';
in
writeShellScriptBin "disk-template" script 