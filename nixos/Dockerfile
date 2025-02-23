# Final Stage: Setup Nix environment
FROM nixos/nix

# Install Nix environment and dependencies from the latest nixpkgs
RUN echo 'extra-experimental-features = nix-command flakes' >> /etc/nix/nix.conf
RUN echo 'extra-platforms = aarch64-linux' >> /etc/nix/nix.conf

# Add nixpkgs-unstable channel
RUN nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs-unstable && nix-channel --update

# Install shell and core utilities
RUN nix-env -iA \
    nixpkgs.fish \
    nixpkgs.zsh \
    nixpkgs.vim \
    nixpkgs.gnused \
    nixpkgs.ncurses \
    nixpkgs.util-linux

# Install network and sync utilities
RUN nix-env -iA \
    nixpkgs.rrsync \
    nixpkgs.rsync \
    nixpkgs.iputils \
    nixpkgs.curl

RUN nix-env -iA nixpkgs.deploy-rs
RUN nix-env -if https://github.com/zhaofengli/colmena/tarball/main
RUN nix-env -iA nixpkgs.nixops_unstable_minimal

WORKDIR /workdir