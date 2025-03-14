# Install k3s on ubuntu

## Flash image

```bash 
# Download image
https://firmware.turingpi.com/turing-rk1/ubuntu_22.04_rockchip_linux/

# Uncompress image
xz -d ubuntu-22.04.3-preinstalled-server-arm64-turing-rk1_v1.33.img.xz

# Flash image
tpi flash -i ~/Downloads/ubuntu-22.04.3-preinstalled-server-arm64-turing-rk1_v1.33.img -n 1
```