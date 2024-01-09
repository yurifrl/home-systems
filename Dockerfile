# Stage 1: QEMU for cross-platform support
FROM multiarch/qemu-user-static:x86_64-aarch64 as qemu

# Final Stage: Setup Nix environment
FROM nixos/nix

# Copy QEMU binary for ARM architecture
COPY --from=qemu /usr/bin/qemu-aarch64-static /usr/bin

WORKDIR /workdir
ENV PATH="/result/bin:/usr/local/bin:$PATH"

# Configure Nix for experimental features and extra platforms
RUN echo 'extra-experimental-features = nix-command flakes' >> /etc/nix/nix.conf
RUN echo 'extra-platforms = aarch64-linux' >> /etc/nix/nix.conf

# Install packages using Nix
RUN nix-env -f https://github.com/nix-community/nixos-generators/archive/master.tar.gz -i

# Update the Nix channel
RUN nix-channel --update

# TODO: This should be in a flake
RUN nix-env -i vim fish go nixops-unstable nixpkgs-fmt

#
COPY go.mod go.sum ./
RUN go mod tidy
#
COPY . .
# Maybe this should be in a flake to
RUN go build -o /usr/local/bin/hs

# Set the default command
ENTRYPOINT ["hs"]
CMD ["help"]
