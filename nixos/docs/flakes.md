
### Building and Deploying
```bash
# Check configuration
nix flake check

# Build AMD and Intel images
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux.default --impure

# Build Pi image
nix build .#images.rpi --impure

# Deploy everywhere
docker compose run --rm deploy . -- --impure

# Switch configuration on Pi
nixos-rebuild switch --flake .#rpi --impure --show-trace 
```
