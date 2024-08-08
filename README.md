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

sudo nixos-rebuild switch --flake .#rpi --impure --show-trace 

# TODO
- [ ] Make so that the system never comes up without tailscale


## References
- [Multicast DNS - Wikipedia](https://en.wikipedia.org/wiki/Multicast_DNS)
- [Zero-configuration networking - Wikipedia](https://en.wikipedia.org/wiki/Zero-configuration_networking#DNS-SD)
- [BMC API](https://docs.turingpi.com/docs/turing-pi2-bmc-api#flash--firmware)
- [Storage](https://docs.turingpi.com/docs/turing-pi2-kubernetes-cluster-storage#option-2-the-longhorn)
