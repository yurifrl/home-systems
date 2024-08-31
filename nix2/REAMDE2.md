
```
nix flake check


```
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux.default --impure
nix build .#packages.aarch64-linux --impure