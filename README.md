# Home Systems

Build the image, boot it, kubernetes starts, turn on the lights.

## Notes

nix-build '<nixpkgs/nixos>' -A config.system.build.sdImage -I nixos-config=./sd-image.nix --argstr system aarch64-linux

cp ./result/sd-image/*.img* .

search packages

nix search nixpkgs cowsay   

## Nix Emergency Kit

You are going to leave this running for you know how long, you will forget how to do basic stuff

# TO inspect current system?

nix eval --raw --impure --expr "builtins.currentSystem"


```
COPY nix/ .

RUN nix \
    --option filter-syscalls false \
    --show-trace \
    build

RUN mv result /result
```
### 

https://nixos.wiki/wiki/NixOS_configuration_editors

Nix packages discovery
```nix
❯ nix repl
nix-repl> :l . # In a flake
nix-repl> nixopsConfigurations.default.network.storage.legacy # Then you can look at stuff
```
#
nixops info -d v7

# Errors

- Git tree is dirty
  - Commit everything, cleanup stash
  - `set -Ux NIX_GIT_CHECKS false` To disable this behaviout


# TODO

- [ ] Config nix to run as nobody
- [ ] Install Tailscale
  - [ ] On tailscale, have one network for admin, one for people, and one for iot, maybe have multiple for iot, cameras, I dont know, have to master that
  - [ ] 
#
Build nix flake

# Reference

Sure, let's categorize these links into relevant groups:

### NixOS on Raspberry Pi
- [davegallant/nixos-pi: NixOS configuration and OS image builder (builds for the Raspberry Pi)](https://github.com/davegallant/nixos-pi)
- [dfrankland/nixos-rpi-sd-image: A convenient way to create custom Raspberry Pi NixOS SD images.](https://github.com/dfrankland/nixos-rpi-sd-image/tree/main)
- [hugolgst/nixos-raspberry-pi-cluster: A user-guide to create a Raspberry Pi (3B+, 4) cluster under NixOS and managed by NixOps](https://github.com/hugolgst/nixos-raspberry-pi-cluster/tree/master)
- [Installing NixOS on Raspberry Pi 4](https://mtlynch.io/nixos-pi4/)
- [NixOS on ARM/Raspberry Pi 4 - NixOS Wiki](https://nixos.wiki/wiki/NixOS_on_ARM/Raspberry_Pi_4)
- [NixOS on a Raspberry Pi: creating a custom SD image with OpenSSH out of the box | Roberto Frenna](https://rbf.dev/blog/2020/05/custom-nixos-build-for-raspberry-pis/#nixos-on-a-raspberry-pi)

### NixOS Image Builders
- [nix-community/nixos-generators: Collection of image builders [maintainer=@Lassulus]](https://github.com/nix-community/nixos-generators)

### NixOS Management and Deployment
- [First steps in NixOps, with Flakes](https://github.com/akavel/garden/blob/main/%40seed/20230830-%40nixops-howto.%40flakes.md)
- [Goodbye Kubernetes](https://xeiaso.net/blog/backslash-kubernetes-2021-01-03/)
- [Deploying with GitHub Actions and more Nix](https://thewagner.net/blog/2020/12/06/deploying-with-github-actions-and-more-nix/)
- [Paranoid NixOS Setup - Xe Iaso](https://xeiaso.net/blog/paranoid-nixos-2021-07-18/)
[wmertens comments on Lollypops - simple, parallel, stateless NixOS deployment tool](https://old.reddit.com/r/NixOS/comments/vnajkg/lollypops_simple_parallel_stateless_nixos/ie7afdo/)

### Flakes and Flake Utilities
- [Why you don't need flake-utils · ayats.org](https://ayats.org/blog/no-flake-utils/)
- [Flakes - MyNixOS](https://mynixos.com/flakes)
- [Introduction - flake-parts](https://flake.parts/)
- [garden/@seed/20230830-@nixops-howto.@flakes.md at main · akavel/garden](https://github.com/akavel/garden/blob/main/@seed/20230830-@nixops-howto.@flakes.md)

### Nix with Docker
- [Using Nix with Dockerfiles](https://mitchellh.com/writing/nix-with-dockerfiles)
- [Building container images with Nix](https://thewagner.net/blog/2021/02/25/building-container-images-with-nix/)


## TODO Sort

- [Old tutorial but very complete](https://github.com/illegalprime/nixos-on-arm)
- [Same problem of no machine](https://github.com/NixOS/nixops/issues/1477)
- [Simple nixops example](https://github.com/NixOS/nixpkgs/blob/master/nixos/tests/nixops/legacy/nixops.nix)
- [Robertof - Build custom SD images of NixOS for your Raspberry Pi.](https://github.com/Robertof/nixos-docker-sd-image-builder)
    - This is the most recent and most complete build process I have found yet.

- https://stackoverflow.com/questions/62957306/nixops-how-to-deploy-to-an-existing-nixos-vm
- https://nix-community.github.io/awesome-nix/
- [The Nix Hour #29 [Python libraries in overlays, switching to home-manager on Ubuntu]](https://www.youtube.com/watch?v=pP1bnQwomDg)

- [LlamaIndex](https://docs.llamaindex.ai/en/stable/getting_started/starter_example.html)


https://tailscale.com/kb/1281/app-connectors


https://tailscale.com/kb/1096/nixos-minecraft

https://www.thedroneely.com/posts/nixops-towards-the-final-frontier/
  - Talks about user management and secrets

https://elvishjerricco.github.io/2018/06/24/secure-declarative-key-management.html
  - Dont know the gist here, but it's about secrets

https://docs.hercules-ci.com/hercules-ci/

https://nixops.readthedocs.io/en/latest/overview.html#managing-keys
https://www.thedroneely.com/posts/nixops-towards-the-final-frontier/
https://blog.sekun.net/posts/manage-secrets-in-nixos/
https://github.com/Mic92/sops-nix#setting-a-users-password
https://lgug2z.com/articles/handling-secrets-in-nixos-an-overview/

[Automagically assimilating NixOS machines into your Tailnet with Terraform - Xe Iaso](https://xeiaso.net/blog/nix-flakes-terraform/)
# TODO

- [ ] Config nix to run as nobody
- [ ] Install Tailscale
  - [ ] On tailscale, have one network for admin, one for people, and one for iot, maybe have multiple for iot, cameras, I dont know, have to master that
  - [ ] 


- Local Workflow
- New node
  - `hs new-sd` flash the image localy, using docker
  - goal is to do something like `hs flash`, and go prompt by prombt
  - 3 images found, want to build a new one? y or number to reuse
  - It also offerts to download from github artifacts
  - this parameter can be passed as option parameters -n1, -nx creates new version
  - .img is built inside docker image, and the output will spill to the system to /etc/home-systems/isos
  - next it will ask if you want to flash the image, it will prompt you to choose an device
  - here will be some devices black listed, preferably only sds will show here
  - in iteractive mode, this is will ofer retry in case o failure
  - the target device, can also be passed with a -fd
  - QUESTION: can I make a .img that contains a secret?
- After flash
  -  

- Automated
  - On github actions or other ci, it does the build locally, (preferably it will run on a arm image)
- This will generate an artifact and put it on a registry

- Runtime

- Usage
- hs docker run
- copy ~/.ssh to ~/.ssh in the container

```bash
# Host
cp -r ~/.ssh ssh

# Container
rm -rf ~/.ssh
cp -r ./ssh ~/.ssh
chmod 700 ~/.ssh
chmod 600 ~/.ssh/*
```
