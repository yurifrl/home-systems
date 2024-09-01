Before anything, this is in the for front of my mind: https://github.com/m1dugh/nix-cluster/tree/master


# Home Systems


```
op item get "Home Server" --fields "private key" --reveal > secrets/id_ed25519

nix flake check

# Build amd an intel images
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux.default --impure

# Deploy everywhere
docker compose run --rm deploy . -- --impure
```

# Developing on pi

Copy your id_ed25519 to the pi

clone this repo in the rpi

```bash
sudo nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs-unstable
sudo nix-channel --update

sudo nixos-rebuild switch --flake .#rpi --impure --show-trace 

sudo nixos-rebuild switch --flake .#rpi --impure --show-trace -I nixpkgs-unstable=https://nixos.org/channels/nixpkgs-unstable
```

# Kuubernetes the hard way

- https://github.com/m1dugh/nix-cluster/tree/master

# TODO
- [ ] Make so that the system never comes up without tailscale


## References
- [Multicast DNS - Wikipedia](https://en.wikipedia.org/wiki/Multicast_DNS)
- [Zero-configuration networking - Wikipedia](https://en.wikipedia.org/wiki/Zero-configuration_networking#DNS-SD)
- [BMC API](https://docs.turingpi.com/docs/turing-pi2-bmc-api#flash--firmware)
- [Storage](https://docs.turingpi.com/docs/turing-pi2-kubernetes-cluster-storage#option-2-the-longhorn)
- `nix build ./nix/#nixosConfigurations.rpi.config.system.build.sdImage --show-trace --print-out-paths --no-link --json --impure`

sudo dd bs=4M if=./dist/nixos-sd-image-24.11.20240731.9f918d6-aarch64-linux.img of=/dev/disk conv=fsync status=progress
diskutil unmountDisk /dev/disk5
diskutil list