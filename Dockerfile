# Stage 1: QEMU for cross-platform support
FROM multiarch/qemu-user-static:x86_64-aarch64 as qemu

# Stage 2: Build cli
FROM golang:alpine as build
WORKDIR /src

#
ENV GOMODCACHE /go/pkg/mod/  

# Cache Go modules
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN go build -o /bin/hs

# Final Stage: Setup Nix environment
FROM nixos/nix

# Copy QEMU binary for ARM architecture
COPY --from=qemu /usr/bin/qemu-aarch64-static /usr/bin

# Install Nix environment and dependencies
RUN nix-env -f https://github.com/nix-community/nixos-generators/archive/master.tar.gz -i

RUN echo 'extra-experimental-features = nix-command flakes' >> /etc/nix/nix.conf
RUN echo 'extra-platforms = aarch64-linux' >> /etc/nix/nix.conf

RUN nix-channel --update

## If we cache /nix/store ina volume, everything here will be deleated
RUN nix-env -iA \
    nixpkgs.fish \
    nixpkgs.go \
    nixpkgs.vim \
    nixpkgs.nixpkgs-fmt \
    nixpkgs.gnused \
    nixpkgs.ncurses \
    nixpkgs.rrsync \
    nixpkgs.rsync

RUN nix-env -iA nixpkgs.deploy-rs
RUN nix-env -if https://github.com/zhaofengli/colmena/tarball/main
RUN nix-env -iA nixpkgs.nixops_unstable_minimal

# Copy built CLI binary
COPY --from=build /bin/hs /bin/hs
COPY --from=build /go/pkg/mod /go/pkg/mod

ENV GOMODCACHE /go/pkg/mod/
ENV NIXOPS_STATE=/nixops/deployments.nixops

WORKDIR /src

VOLUME [ "/nixops", "gomod-cache" ]

# Set the default command
ENTRYPOINT ["/bin/hs"]
CMD ["help"]
