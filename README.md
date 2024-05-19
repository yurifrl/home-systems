# Home Systems

## Overview

This guide provides a comprehensive approach to setting up, managing, and troubleshooting your NixOS home system. Whether you're using a Raspberry Pi or a different ARM device, this guide will help you through the process of building, deploying, and managing your NixOS setup.

# Cli

The cli needs to run in its docker image to work properly.

```bash

docker compose run --rm hs

hs run

```

- `hs nix build`

# Building and Booting the Image

## Building the Image
To build the NixOS image for an ARM device (e.g., Raspberry Pi), use the following command:
```bash
nix-build '<nixpkgs/nixos>' -A config.system.build.sdImage -I nixos-config=./sd-image.nix --argstr system aarch64-linux
```
After building, copy the image:
```bash
cp ./result/sd-image/*.img* .
```

## Searching for Packages
You can search for available Nix packages with:
```bash
nix search nixpkgs cowsay
```

## Inspecting the Current System
To inspect the current system, run:
```bash
nix eval --raw --impure --expr "builtins.currentSystem"
```

# NixOS Configuration

## Example Dockerfile
This example Dockerfile demonstrates how to build NixOS in a container:
```Dockerfile
COPY nix/ .

RUN nix \
    --option filter-syscalls false \
    --show-trace \
    build

RUN mv result /result
```

## Configuration Editors
For editing NixOS configurations, refer to the [NixOS configuration editors](https://nixos.wiki/wiki/NixOS_configuration_editors).

## Nix Packages Discovery
```nix
❯ nix repl
nix-repl> :l . # In a flake
nix-repl> fooConfigurations.default.network.storage.legacy # Then you can look at stuff
```

## Terraform Info

# Workflow

## Local Workflow
1. **New Node**:
    - Flash the image locally using Docker with `hs new-sd`.
    - Example prompts during `hs flash`:
        - Build a new image or reuse an existing one.
        - Download from GitHub artifacts if available.
        - Provide options for image parameters and device selection.
    - Note: The target device can be specified with `-fd`.

## After Flash
- Configure the newly flashed device.

## Automated Workflow
- On GitHub Actions or other CI, build the image locally (preferably on an ARM image).
- Generate an artifact and upload it to a registry.

## Usage
- Run NixOS in a Docker container with SSH access:
    ```bash
    # Host
    cp -r ~/.ssh ssh

    # Container
    rm -rf ~/.ssh
    cp -r ./ssh ~/.ssh
    chmod 700 ~/.ssh
    chmod 600 ~/.ssh/*
    ```

# Troubleshooting

## Common Errors
- **Git tree is dirty**:
  - Commit everything and clean up the stash.
  - Disable this behavior with:
    ```bash
    set -Ux NIX_GIT_CHECKS false
    ```

# TODO

- [ ] Configure Nix to run as nobody.
- [x] Install Tailscale:
  - [ ] Set up multiple networks for admin, users, and IoT devices.
  - [ ] Find a way to not add the secret to the artifact

# Notes

- The idea is to have one image, and patch it with new things
- But, at the same time edit the same original image file
- so if you genarate a new image, it will be updated
  
# References Links

## NixOS on Raspberry Pi
- [davegallant/nixos-pi: NixOS configuration and OS image builder (builds for the Raspberry Pi)](https://github.com/davegallant/nixos-pi)
- [dfrankland/nixos-rpi-sd-image: A convenient way to create custom Raspberry Pi NixOS SD images.](https://github.com/dfrankland/nixos-rpi-sd-image/tree/main)
- [Installing NixOS on Raspberry Pi 4](https://mtlynch.io/nixos-pi4/)
- [NixOS on ARM/Raspberry Pi 4 - NixOS Wiki](https://nixos.wiki/wiki/NixOS_on_ARM/Raspberry_Pi_4)
- [NixOS on a Raspberry Pi: creating a custom SD image with OpenSSH out of the box | Roberto Frenna](https://rbf.dev/blog/2020/05/custom-nixos-build-for-raspberry-pis/#nixos-on-a-raspberry-pi)

## NixOS Image Builders
- [nix-community/nixos-generators: Collection of image builders [maintainer=@Lassulus]](https://github.com/nix-community/nixos-generators)

## NixOS Management and Deployment
- [Goodbye Kubernetes](https://xeiaso.net/blog/backslash-kubernetes-2021-01-03/)
- [Deploying with GitHub Actions and more Nix](https://thewagner.net/blog/2020/12/06/deploying-with-github-actions-and-more-nix/)
- [Paranoid NixOS Setup - Xe Iaso](https://xeiaso.net/blog/paranoid-nixos-2021-07-18/)
- [wmertens comments on Lollypops - simple, parallel, stateless NixOS deployment tool](https://old.reddit.com/r/NixOS/comments/vnajkg/lollypops_simple_parallel_stateless_nixos/ie7afdo/)

## Flakes and Flake Utilities
- [Why you don't need flake-utils · ayats.org](https://ayats.org/blog/no-flake-utils/)
- [Flakes - MyNixOS](https://mynixos.com/flakes)
- [Introduction - flake-parts](https://flake.parts/)

## Nix with Docker
- [Using Nix with Dockerfiles](https://mitchellh.com/writing/nix-with-dockerfiles)
- [Building container images with Nix](https://thewagner.net/blog/2021/02/25/building-container-images-with-nix/)

## Various Useful Links
- [Old tutorial but very complete](https://github.com/illegalprime/nixos-on-arm)
- [Robertof - Build custom SD images of NixOS for your Raspberry Pi.](https://github.com/Robertof/nixos-docker-sd-image-builder)
- [Nix Community - Awesome Nix](https://nix-community.github.io/awesome-nix/)
- [The Nix Hour #29 [Python libraries in overlays, switching to home-manager on Ubuntu]](https://www.youtube.com/watch?v=pP1bnQwomDg)

## Additional Resources
- [LlamaIndex](https://docs.llamaindex.ai/en/stable/getting_started/starter_example.html)
- [Tailscale App Connectors](https://tailscale.com/kb/1281/app-connectors)
- [NixOS Minecraft](https://tailscale.com/kb/1096/nixos-minecraft)
- [Secure Declarative Key Management](https://elvishjerricco.github.io/2018/06/24/secure-declarative-key-management.html)
- [Hercules CI Documentation](https://docs.hercules-ci.com/hercules-ci/)
- [Managing Secrets in NixOS](https://blog.sekun.net/posts/manage-secrets-in-nixos/)
- [Handling Secrets in NixOS](https://github.com/Mic92/sops-nix#setting-a-users-password)
- [Handling Secrets in NixOS - Overview](https://lgug2z.com/articles/handling-secrets-in-nixos-an-overview/)
- [Automagically assimilating NixOS machines into your Tailnet with Terraform - Xe Iaso](https://xeiaso.net/blog/nix-flakes-terraform/)