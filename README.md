# Home Systems


```
nix flake check

# Build amd an intel images
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux 

# Deploy everywhere
docker compose run --rm colmena apply --impure

# Deploy only on the vm
docker compose run --rm colmena apply --impure --on vm
```

