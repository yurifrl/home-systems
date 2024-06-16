
# Jun 16 2024

- Derivations
  - Works: `nix-build -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'`
  - `nix-build cowsay-version.nix --arg cowsay '(import <nixpkgs> {}).cowsay' --arg stdenv '(import <nixpkgs> {}).stdenv'`
  - `nix-instantiate -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'`  


```sh
nix build .#iso.config.system.build.isoImage


nix build --rebuild --impure --builders 'ssh://nixos@192.168.68.108' ./nix/#nixosConfigurations.rpi.config.system.build.sdImage

NIX_SSHOPTS="-A" nixos-rebuild switch --flake ./nix/#nixosConfigurations.rpi.config.system.build.sdImage --target-host ssh://nixos@192.168.68.108 --use-remote-sudo

nix run github:serokell/deploy-rs nix/
```

- Dev and Build
  - nix flake check
  - nix build .#packages.aarch64-linux.default .#packages.x86_64-linux 

- Colmena
  - https://sourcegraph.com/github.com/Cottand/selfhosted@6ddede91264e7d1f3eb627d35983e2e7743761bd/-/blob/flake.nix?L30:7-30:18
  - docker compose run --rm colmena apply --impure --on vm

# Next
