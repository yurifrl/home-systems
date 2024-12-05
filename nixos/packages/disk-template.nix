{ stdenv, bash, util-linux, e2fsprogs, systemd, findutils, gnugrep, coreutils, gnused, writeShellScriptBin }:

let
  # Define disk layouts as a list instead of an attribute set
  diskLayouts = [
    {
      name = "USB_SanDisk_3.2Gen1_04016ae56ccfac4811a0010dddda218064618aee1065dfdc49eb60ee9f9161eba17a00000000000000000000ef6dbef8ff077b1867558107ab286235-0:0";
      content = ''
        label: gpt
        unit: sectors

        /dev/sda1 : start=2048, size=2097152, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4
      '';
    }
    {
      name = "Kingston_DataTraveler_3.0_408D5CBF5F0AE830A9150618-0:0";
      content = ''
        label: gpt
        unit: sectors

        /dev/sdb1 : start=2048, size=31457280, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4
      '';
    }
  ];

  # Create a temporary directory with the layout files
  createLayoutFiles = ''
    # Create temporary directory for disk layouts
    TEMP_DIR=$(mktemp -d)
    
    # Copy built-in layouts
    ${builtins.concatStringsSep "\n" (map (layout: ''
      echo '${layout.content}' > "$TEMP_DIR/${layout.name}.sfdisk"
    '') diskLayouts)}
    
    # If user specified a directory, copy its .sfdisk files
    if [ -n "$1" ]; then
      cp "$1"/*.sfdisk "$TEMP_DIR/" 2>/dev/null || true
    fi
  '';

  showCommands = ''
    # Prompt for disk selection
    echo "=== Available Disks ==="
    ${util-linux}/bin/lsblk -d -o NAME,SIZE,MODEL,SERIAL | ${gnugrep}/bin/grep -v "loop"
    echo ""

    read -p "Enter disk name to show commands for (e.g., sda): " DISK_NAME
    if [ -z "$DISK_NAME" ]; then
        echo "No disk selected. Exiting."
        exit 1
    fi

    echo "Manual Command Reference for /dev/$DISK_NAME:"
    echo "------------------------"
    echo "1. List all disks:"
    echo "   lsblk -d -o NAME,SIZE,MODEL,SERIAL"
    echo ""
    echo "2. Get disk info and serial (used for template matching):"
    echo "   lsblk -o NAME,SERIAL,MODEL /dev/$DISK_NAME | sed 1d"
    echo "   # Get just the serial:"
    echo "   lsblk -o NAME,SERIAL,MODEL /dev/$DISK_NAME | sed 1d | tr -s ' ' | cut -d' ' -f2"
    echo ""
    echo "3. Find matching template file:"
    echo "   find /path/to/templates -name \"*.sfdisk\" -type f | grep -F \"SERIAL_NUMBER\""
    echo ""
    echo "4. Show current partition layout:"
    echo "   sudo fdisk -l /dev/$DISK_NAME"
    echo ""
    echo "5. Wipe filesystem signatures:"
    echo "   sudo wipefs -a /dev/$DISK_NAME"
    echo ""
    echo "6. Apply partition template (replace TEMPLATE.sfdisk with your template file):"
    echo "   sudo sfdisk --force /dev/$DISK_NAME < TEMPLATE.sfdisk"
    echo ""
    echo "7. Unmount partition:"
    echo "   sudo umount /dev/''${DISK_NAME}1"
    echo ""
    echo "8. Format partition as ext4:"
    echo "   sudo mkfs.ext4 -F /dev/''${DISK_NAME}1"
    echo ""
    echo "9. Find disk templates:"
    echo "   find /path/to/templates -name '*.sfdisk' -type f"
    echo ""
    exit 0
  '';

  showHelp = ''
    echo "Usage: disk-template -a [-d directory] [-h] [-t]"
    echo ""
    echo "Options:"
    echo "  -a             Apply disk template (required to run)"
    echo "  -d directory   Specify directory to search for disk templates (default: /etc/disk-templates)"
    echo "  -h             Display this help message"
    echo "  -t             List manual commands for troubleshooting"
    echo ""
    echo "Available templates:"
    
    # Check if directory exists and contains .sfdisk files
    if [ -d "$TEMP_DIR" ]; then
      template_count=$(${findutils}/bin/find "$TEMP_DIR" -name "*.sfdisk" -type f | wc -l)
      if [ "$template_count" -gt 0 ]; then
        echo "Templates in $TEMP_DIR:"
        for template in "$TEMP_DIR"/*.sfdisk; do
          if [ -f "$template" ]; then
            echo "  - $(${coreutils}/bin/basename "$template" .sfdisk)"
            echo "    Layout:"
            ${gnused}/bin/sed 's/^/      /' "$template"
            echo ""
          fi
        done
      else
        echo "  No templates found in $TEMP_DIR"
      fi
    else
      echo "  Directory $TEMP_DIR does not exist"
    fi
    exit 0
  '';

  mainScript = ''
    #!/usr/bin/env bash

    # Check if script is running with sudo
    if [ "$EUID" -ne 0 ]; then
      echo "Error: This script must be run with sudo"
      exit 1
    fi

    # Parse command line arguments
    USER_DIR=""
    APPLY=0

    while getopts "ad:ht" opt; do
      case $opt in
        a)
          APPLY=1
          ;;
        d)
          USER_DIR="$OPTARG"
          ;;
        h)
          ${showHelp}
          ;;
        t)
          ${showCommands}
          ;;
        \?)
          echo "Invalid option: -$OPTARG" >&2
          echo "Use -h for help" >&2
          exit 1
          ;;
      esac
    done

    # Show help if no arguments or -a not specified
    if [ $APPLY -eq 0 ]; then
      show_help
    fi

    # Create layout files in temporary directory
    echo "Creating layout files..."
    ${createLayoutFiles} "$USER_DIR"

    # List all available disks
    echo "=== Available Disks ==="
    ${util-linux}/bin/lsblk -d -o NAME,SIZE,MODEL,SERIAL | ${gnugrep}/bin/grep -v "loop"
    echo ""

    # Prompt for disk selection
    read -p "Enter disk name to partition (e.g., sda): " DISK_NAME
    if [ -z "$DISK_NAME" ]; then
        echo "No disk selected. Exiting."
        exit 1
    fi

    # Get the full disk info (replacing udevadm)
    DISK_INFO=$(${util-linux}/bin/lsblk -o NAME,SERIAL,MODEL /dev/''${DISK_NAME} | ${gnused}/bin/sed 1d)
    DISK_SERIAL=$(echo "$DISK_INFO" | ${coreutils}/bin/tr -s ' ' | ${coreutils}/bin/cut -d' ' -f2)

    # Look for existing layout file that matches this disk
    LAYOUT_FILE=""
    if [ -n "$DISK_SERIAL" ]; then
      LAYOUT_FILE=$(${findutils}/bin/find "$TEMP_DIR" -name "*.sfdisk" -type f | ${gnugrep}/bin/grep -F "$DISK_SERIAL" || true)
    fi

    if [ -z "$LAYOUT_FILE" ]; then
        echo "No layout file found for this disk"
        echo "Current disk info:"
        echo "Model: $(echo "$DISK_INFO" | ${coreutils}/bin/tr -s ' ' | ${coreutils}/bin/cut -d' ' -f3-)"
        echo "Diskname: $DISK_NAME"
        echo "Diskinfo: $DISK_INFO"
        echo "Serial: $DISK_SERIAL"
        echo "Expected filename pattern: *''${DISK_SERIAL}*.sfdisk"
        echo "Searched in: $TEMP_DIR"
        echo "Available templates:"
        ls -l "$TEMP_DIR"/*.sfdisk 2>/dev/null || echo "  No .sfdisk files found"
        exit 1
    fi

    # Display disk information
    echo "================================================================"
    echo "                     === Disk Information ===                     "
    echo "================================================================"
    echo "Device: /dev/''${DISK_NAME}"
    echo "Model: $(echo "$DISK_INFO" | ${coreutils}/bin/tr -s ' ' | ${coreutils}/bin/cut -d' ' -f3-)"
    echo "Serial: $DISK_SERIAL"
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
    read -p "Are you sure you want to proceed? (y/n) " answer
    if [ "$answer" != "y" ]; then
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
    echo "Unmounting partition if mounted..."
    ${util-linux}/bin/umount "/dev/''${DISK_NAME}1" 2>/dev/null || true

    echo "Formatting /dev/''${DISK_NAME}1..."
    ${e2fsprogs}/bin/mkfs.ext4 -F /dev/''${DISK_NAME}1
    echo "================================================================"

    echo "================================================================"
    echo "                     === Final Disk State ===                     "
    echo "================================================================"
    echo "Partition table:"
    echo "----------------------------------------------------------------"
    ${util-linux}/bin/fdisk -l /dev/''${DISK_NAME}
    echo ""
    echo "Filesystem details:"
    echo "----------------------------------------------------------------"
    ${util-linux}/bin/lsblk -o NAME,SIZE,FSTYPE,MOUNTPOINT /dev/''${DISK_NAME}
    echo "================================================================"
    echo "Partitioning complete!"
  '';
in
writeShellScriptBin "disk-template" mainScript 