# Use Alpine as the base image
FROM alpine:latest

# Install Nix
RUN apk add --no-cache curl xz \
    && echo "https://nixos.org/releases/nix/nix-2.3.10/install" | xz | sh

# Add Nix to PATH
ENV PATH=/root/.nix-profile/bin:$PATH

# Copy the default.nix file (or other nix build file)
#COPY default.nix /tmp/default.nix

# Set working directory
#WORKDIR /tmp

# Build with nix-build
#RUN nix-build default.nix -o result

# Set result as the only output of the image
#CMD ["cp", "-r", "result", "/out"]

