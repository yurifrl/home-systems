{ stdenv, bash, util-linux, e2fsprogs, udev, findutils, gnugrep, coreutils, gnused, writeShellScriptBin }:

let
  # Define disk layouts as a list instead of an attribute set
  diskLayouts = [
    {
      name = "USB_SanDisk_3.2Gen1_04016ae56ccfac4811a0010dddda218064618aee1065dfdc49eb60ee9f9161eba17a00000000000000000000ef6dbef8ff077b1867558107ab286235-0:0";
      content = ''
        label: gpt
        unit: sectors

        /dev/sda1 : start=2048, size=2097152, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4
        /dev/sda2 : type=0FC63DAF-8483-4772-8E79-3D69D8477DE4
      '';
    }
  ];

  # Create a temporary directory with the layout files
  createLayoutFiles = ''
    mkdir -p disks
    ${builtins.concatStringsSep "\n" (map (layout: ''
      echo '${layout.content}' > disks/${layout.name}.sfdisk
    '') diskLayouts)}
  '';

  script = ''
    #!/usr/bin/env bash

    # Check for root privileges
    if [ "$(id -u)" -ne 0 ]; then
      echo "Error: This script must be run with sudo privileges"
      exit 1
    fi

    # Display help message
    show_help() {
      echo "Usage: disk-template -a [-d directory] [-h] [-l]"
      echo ""
      echo "Options:"
      echo "  -a             Apply disk template (required to run)"
      echo "  -d directory   Specify directory to search for disk templates (default: /etc/disk-templates)"
      echo "  -h             Display this help message"
      echo "  -l             List manual commands for troubleshooting"
      echo ""
      echo "Available templates:"
      
      # Check if directory exists and contains .sfdisk files
      if [ -d "$SEARCH_DIR" ]; then
        template_count=$(${findutils}/bin/find "$SEARCH_DIR" -name "*.sfdisk" -type f | wc -l)
        if [ "$template_count" -gt 0 ]; then
          echo "Templates in $SEARCH_DIR:"
          for template in "$SEARCH_DIR"/*.sfdisk; do
            if [ -f "$template" ]; then
              echo "  - $(${coreutils}/bin/basename "$template" .sfdisk)"
              echo "    Layout:"
              ${gnused}/bin/sed 's/^/      /' "$template"
              echo ""
            fi
          done
        else
          echo "  No templates found in $SEARCH_DIR"
        fi
      else
        echo "  Directory $SEARCH_DIR does not exist"
      fi
      exit 0
    }

    # Parse command line arguments
    SEARCH_DIR="."
    APPLY=0

    while getopts "ad:hl" opt; do
      case $opt in
        a)
          APPLY=1
          ;;
        d)
          SEARCH_DIR="$OPTARG"
          ;;
        h)
          show_help
          ;;
        l)
          show_commands
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

    # Create layout files in specified directory
    mkdir -p "$SEARCH_DIR/disks"
    ${builtins.concatStringsSep "\n" (map (layout: ''
      echo '${layout.content}' > "$SEARCH_DIR/disks/${layout.name}.sfdisk"
    '') diskLayouts)}

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

    # Get the full disk info
    DISK_INFO=$(${udev}/bin/udevadm info --query=property --name=/dev/''${DISK_NAME})

    # Look for existing layout file that matches this disk
    LAYOUT_FILE=$(${findutils}/bin/find "$SEARCH_DIR/disks" -name "*.sfdisk" -type f | ${gnugrep}/bin/grep -F "$(echo "$DISK_INFO" | ${gnugrep}/bin/grep ID_SERIAL= | cut -d= -f2)")

    if [ ! -f "$LAYOUT_FILE" ]; then
        echo "No layout file found for this disk"
        echo "Current disk info:"
        echo "$DISK_INFO" | ${gnugrep}/bin/grep -E "ID_SERIAL=|ID_MODEL="
        exit 1
    fi

    # Display disk information
    echo "================================================================"
    echo "                     === Disk Information ===                     "
    echo "================================================================"
    echo "Device: /dev/''${DISK_NAME}"
    echo "Model: $(echo "$DISK_INFO" | ${gnugrep}/bin/grep ID_MODEL= | cut -d= -f2)"
    echo "Serial: $(echo "$DISK_INFO" | ${gnugrep}/bin/grep ID_SERIAL= | cut -d= -f2)"
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
    echo "Unmounting partition if mounted..."
    ${util-linux}/bin/umount "/dev/''${DISK_NAME}1" 2>/dev/null || true

    echo "Formatting /dev/''${DISK_NAME}1..."
    ${e2fsprogs}/bin/mkfs.ext4 -F /dev/''${DISK_NAME}1
    echo "================================================================"

    # Display commands for manual troubleshooting
    show_commands() {
      echo "Manual Command Reference:"
      echo "------------------------"
      echo "1. List all disks:"
      echo "   lsblk -d -o NAME,SIZE,MODEL,SERIAL"
      echo ""
      echo "2. Get disk info (replace sdX with your disk):"
      echo "   sudo udevadm info --query=property --name=/dev/sdX"
      echo ""
      echo "3. Show current partition layout:"
      echo "   sudo fdisk -l /dev/sdX"
      echo ""
      echo "4. Wipe filesystem signatures:"
      echo "   sudo wipefs -a /dev/sdX"
      echo ""
      echo "5. Apply partition template (replace TEMPLATE.sfdisk with your template file):"
      echo "   sudo sfdisk --force /dev/sdX < TEMPLATE.sfdisk"
      echo ""
      echo "6. Unmount partition:"
      echo "   sudo umount /dev/sdX1"
      echo ""
      echo "7. Format partition as ext4:"
      echo "   sudo mkfs.ext4 -F /dev/sdX1"
      echo ""
      echo "8. Find disk templates:"
      echo "   find /path/to/templates -name '*.sfdisk' -type f"
      echo ""
      echo "Note: Replace 'sdX' with your actual disk name (e.g., 'sda')"
      exit 0
    }
  '';
in
writeShellScriptBin "disk-template" script 