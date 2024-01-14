# Stage 1: QEMU for cross-platform support
FROM multiarch/qemu-user-static:x86_64-aarch64 as qemu

# Stage 2: Build cli
FROM golang:alpine as gobuilder

WORKDIR /workdir

COPY go.mod go.sum ./
RUN go mod tidy

COPY . .
RUN go build -o hs

# Final Stage: Setup Nix environment
# FROM gcr.io/nixos/nix
FROM nixos/nix

# Copy QEMU binary for ARM architecture
COPY --from=qemu /usr/bin/qemu-aarch64-static /usr/bin

# Is for generation of the sd image
RUN nix-env -f https://github.com/nix-community/nixos-generators/archive/master.tar.gz -i

# Configure Nix for experimental features and extra platforms
RUN echo 'extra-experimental-features = nix-command flakes' >> /etc/nix/nix.conf
RUN echo 'extra-platforms = aarch64-linux' >> /etc/nix/nix.conf

# Update the Nix channel
RUN nix-channel --update

# COPY --from=gobuilder /workdir/hs /usr/local/bin/hs
RUN nix-env -iA nixpkgs.nixopsUnstable nixpkgs.fish nixpkgs.go nixpkgs.vim nixpkgs.nixpkgs-fmt nixpkgs.gnused nixpkgs.ncurses

WORKDIR /workdir

ENV NIXOPS_STATE=/nixops/deployments.nixops

# Set the default command
ENTRYPOINT ["hs"]
CMD ["help"]