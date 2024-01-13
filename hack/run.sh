#!/usr/bin/env bash

VERSION="img-v1"

cd "/home/nixos/image/outputs/$VERSION" || exit

nix-build '<nixpkgs/nixos>' \
    -A config.system.build.sdImage \
    -I nixos-config=/home/nixos/image/config/sd-image.nix \
    --argstr system aarch64-linux