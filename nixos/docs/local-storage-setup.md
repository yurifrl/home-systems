```bash
# Unmount everything
sudo umount -f /dev/sda*

# Wipe and partition in one go
sudo sfdisk /dev/sda --wipe=always <<EOF
label: gpt
device: /dev/sda
unit: sectors

/dev/sda1 : size=4194304, type=83
/dev/sda2 : size=54525952, type=83
EOF

# Format monitor partition only
sudo mkfs.ext4 /dev/sda1

# We don't format sda2 because it needs to be a raw block device for Ceph OSD
```