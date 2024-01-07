# Home Systems

Build the image, boot it, kubernetes starts, turn on the lights.

## References

- [Robertof - Build custom SD images of NixOS for your Raspberry Pi.](https://github.com/Robertof/nixos-docker-sd-image-builder)
    - This is the most recent and most complete build process I have found yet.

## Notes

nix-build '<nixpkgs/nixos>' -A config.system.build.sdImage -I nixos-config=./sd-image.nix --argstr system aarch64-linux

cp ./result/sd-image/*.img* .

### Bulk

- [davegallant/nixos-pi: NixOS configuration and OS image builder (builds for the Raspberry Pi)](https://github.com/davegallant/nixos-pi)
- [dfrankland/nixos-rpi-sd-image: A convenient way to create custom Raspberry Pi NixOS SD images.](https://github.com/dfrankland/nixos-rpi-sd-image/tree/main)
- [hugolgst/nixos-raspberry-pi-cluster: A user-guide to create a Raspberry Pi (3B+, 4) cluster under NixOS and managed by NixOps](https://github.com/hugolgst/nixos-raspberry-pi-cluster/tree/master)
- [Installing NixOS on Raspberry Pi 4](https://mtlynch.io/nixos-pi4/)
- [nix-community/nixos-generators: Collection of image builders [maintainer=@Lassulus]](https://github.com/nix-community/nixos-generators)
- [NixOS on a Raspberry Pi: creating a custom SD image with OpenSSH out of the box | Roberto Frenna](https://rbf.dev/blog/2020/05/custom-nixos-build-for-raspberry-pis/#nixos-on-a-raspberry-pi)
- [NixOS on ARM/Raspberry Pi 4 - NixOS Wiki](https://nixos.wiki/wiki/NixOS_on_ARM/Raspberry_Pi_4)



- [NixOS on ARM/Raspberry Pi 4 - NixOS Wiki](https://nixos.wiki/wiki/NixOS_on_ARM/Raspberry_Pi_4)
  - Nix wiki, talks about pi configs in general, hardware, network, after the install is done, might need to come here



# Cli

pip install pipreqs

pipreqs . --force         

python3 -m pip install -r requirements.txt 



# TODO

- [ ] Config nix to run as nobody

```
  config = {
    User = "nobody";
    Cmd = [ "/bin/sh" "-l" ];
  };
```