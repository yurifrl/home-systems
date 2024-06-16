# Workflow


## New Workflow

### New Image
- Build
  - Using ci
  - Locally with
    - `nix build .#packages.aarch64-linux.default .#packages.x86_64-linux`

### Flash new .img
1. Get the image
2. Flash the image locally using Docker with `hs new-sd`.


### After Flash
Configure the newly flashed device.

## Automated Workflow
1. On GitHub Actions or other CI, build the image locally (preferably on an ARM image).
2. Generate an artifact and upload it to a registry.

# Dev Workdlow

- Dev and Build:
  - `nix flake check`
  - `nix build .#packages.aarch64-linux.default .#packages.x86_64-linux`
- Colmena:
  - [Colmena example flake.nix](https://sourcegraph.com/github.com/Cottand/selfhosted@6ddede91264e7d1f3eb627d35983e2e7743761bd/-/blob/flake.nix?L30:7-30:18)
  - Run Colmena:
    ```bash
    docker compose run --rm colmena apply --impure --on vm
    ```
- Derivations:
  - Works: `nix-build -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'`
  - `nix-build cowsay-version.nix --arg cowsay '(import <nixpkgs> {}).cowsay' --arg stdenv '(import <nixpkgs> {}).stdenv'`
  - `nix-instantiate -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'`
