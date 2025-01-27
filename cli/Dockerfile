# Stage 2: Build cli
FROM golang:alpine as build
WORKDIR /src

ENV GOMODCACHE /go/pkg/mod/

# Cache Go modules
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN go build -o /bin/hs

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

# Install development tools
RUN nix-env -iA \
    nixpkgs.go \
    nixpkgs.nixpkgs-fmt

# Install network and sync utilities
RUN nix-env -iA \
    nixpkgs.rrsync \
    nixpkgs.rsync \
    nixpkgs.iputils \
    nixpkgs.curl

# Install Ansible and SSH
RUN nix-env -iA \
    nixpkgs.ansible \
    nixpkgs.openssh \
    nixpkgs.sshpass

RUN nix-env -iA nixpkgs.deploy-rs
RUN nix-env -if https://github.com/zhaofengli/colmena/tarball/main
RUN nix-env -iA nixpkgs.nixops_unstable_minimal

# Copy built CLI binary
COPY --from=build /bin/hs /bin/hs
COPY --from=build /go/pkg/mod /go/pkg/mod

ENV GOMODCACHE /go/pkg/mod/
ENV PATH=/bin:$PATH

WORKDIR /src

VOLUME [ "gomod-cache" ]

# Set the default command
ENTRYPOINT ["/bin/hs"]
CMD ["help"]
