# Home Systems


```
nix flake check

# Build amd an intel images
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux 

# Deploy everywhere
docker compose run --rm deploy . -- --impure
```

# TODO
- [ ] Make so that the system never comes up without tailscale


## References
- [Multicast DNS - Wikipedia](https://en.wikipedia.org/wiki/Multicast_DNS)
- [Zero-configuration networking - Wikipedia](https://en.wikipedia.org/wiki/Zero-configuration_networking#DNS-SD)
- [BMC API](https://docs.turingpi.com/docs/turing-pi2-bmc-api#flash--firmware)
- [Storage](https://docs.turingpi.com/docs/turing-pi2-kubernetes-cluster-storage#option-2-the-longhorn)
