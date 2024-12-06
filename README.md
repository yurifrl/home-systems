# Home Systems

# Important Concepts

## Daily Routine
```bash
# Switch configuration on Pi
sudo nixos-rebuild switch --flake .#rpi --impure --show-trace

# ArgoCD setup
sudo systemctl restart argo-setup & sudo journalctl -u argo-setup.service -f

# Secret loader
sudo systemctl restart secret-loader & sudo journalctl -u secret-loader.service -f

# Kill node if needed
kill-node.sh
```

## Initial Setup

```bash
# Get private key from 1Password to allow push from the machine
op item get "Home Server" --fields "private key" --reveal > id_ed25519

# copy key to machine
scp id_ed25519 root@nixos-1:/home/nixos/.ssh/id_ed25519

# copy secrets to machine
scp secrets.sh root@nixos-1:/data/secrets.sh

# ssh into machine
ssh nixos@nixos-1

# Switch configuration on Pi
sudo nixos-rebuild switch --flake .#rpi --impure --show-trace

# Add unstable channel (not sure if still needed)
sudo nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs-unstable
sudo nix-channel --update

# clone flake
git clone git@github.com:yurifrl/home-systems.git

# Exit ssh session
exit

# Set up k aliases and permissions
scp root@nixos-1:/etc/rancher/k3s/k3s.yaml ~/.kube/k3s.yaml
set -gx KUBECONFIG /etc/rancher/k3s/k3s.yaml
sudo chmod 644 /etc/rancher/k3s/k3s.yaml
k get nodes
```

## Building and Deployment

### Local Development
```bash
# Build AMD64 and ARM64 images
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux.default --impure

# Check configuration
nix flake check

# Build Raspberry Pi image specifically
nix build .#images.rpi --impure
```

### Deployment Methods
```bash
# Deploy using docker image and deploy-rs
docker compose run --rm deploy . -- --impure

# Direct deployment when SSH'd into Piflake check
nixos-rebuild switch --flake .#rpi --impure --show-trace 

# Copy secrets to Machine
scp hack/secrets.sh root@nixos-1:/data/secrets.sh
```

### Service Management
```bash
# ArgoCD setup
sudo systemctl restart argo-setup & sudo journalctl -u argo-setup.service -f

# Secret loader
sudo systemctl restart secret-loader & sudo journalctl -u secret-loader.service -f

# Kill node if needed
kill-node.sh
```

## Tool Configuration

### Cloudflare Tunnel Setup
```bash
cloudflared tunnel login
cloudflared tunnel create nixos-1
k -n cloudflare-tunnel create secret generic cloudflare-tunnel-secret --from-file=credentials.json=/home/nixos/.
```

### Tailscale Configuration
```bash
# Create secret with Tailscale auth key
k -n tailscale create secret generic tailscale-auth --from-literal=auth-key='tskey-auth-xxxxx'
```

### External DNS Setup
```bash
# Create secret with Cloudflare API token
k -n external-dns create secret generic cloudflare-api-token --from-literal=token='your-cloudflare-api-token'
```

## Useful Commands

```bash
# Search for packages
nix search nixpkgs cowsay

# Inspect the current system
nix eval --raw --impure --expr "builtins.currentSystem"
```


## Image Building
```bash
# Build just the Raspberry Pi image (simplified command)
nix build .#images.rpi --impure

# Build packages for both ARM64 and x86_64 architectures
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux.default --impure

# Build detailed SD card image with debug output (low-level command)
nix build ./nix/#nixosConfigurations.rpi.config.system.build.sdImage --show-trace --print-out-paths --no-link --json --impure

# To build the NixOS image for an ARM device (e.g., Raspberry Pi), use the following command:
nix-build '<nixpkgs/nixos>' -A config.system.build.sdImage -I nixos-config=./sd-image.nix --argstr system aarch64-linux

# Build image on Pi
nix build --rebuild --impure --builders 'ssh://nixos@192.168.68.108' ./nix/#nixosConfigurations.rpi.config.system.build.sdImage

# Switch configuration on Pi
NIX_SSHOPTS="-A" nixos-rebuild switch --flake ./nix/#nixosConfigurations.rpi.config.system.build.sdImage --target-host ssh://nixos@192.168.68.108 --use-remote-sudo

# Copy the built image
cp ./result/sd-image/*.img* .
```

## Additional References

### Derivation Examples
```bash
# Basic derivation build
nix-build -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'

# Build package
nix-build cowsay-version.nix --arg cowsay '(import <nixpkgs> {}).cowsay' --arg stdenv '(import <nixpkgs> {}).stdenv'

# Evaluate derivation
nix-instantiate -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'
```

### Alternative Build Methods
```bash
# Build ISO image
nix build .#iso.config.system.build.isoImage

# Using deploy-rs directly
nix run github:serokell/deploy-rs nix/

# Using colmena
docker compose run --rm colmena apply --impure --on vm
```


# TODO

- [ ] Use ingress
- [ ] Filter alerts
- [ ] Https
- [ ] Transfer domain to cloudflare https://www.melodylee.tech/website-development-blog/transfer-domain-godaddy-to-cloudflare
- [ ] Add state to prometheus
- [ ] Add state to grafana
- [ ] Add state to home-assistant

## Old TODO

- [ ] Configure Nix to run as nobody.
- [x] Install Tailscale:
  - [ ] Set up multiple networks for admin, users, and IoT devices.
  - [ ] Find a way to not add the secret to the artifact.
- [-] Pass secrets on dd?
- [x] Build raspberry image with preinstalled software:
  - [Read more](https://discourse.nixos.org/t/build-raspberry-image-with-preinstalled-software/33055/3)
- [ ] Read more on:
  - [Flake does not provide attribute - Help - NixOS Discourse](https://discourse.nixos.org/t/flake-does-not-provide-attribute/32156/2)
  - [Practical Nix flake anatomy: a guided tour of flake.nix | Vladimir Timofeenko's blog](https://vtimofeenko.com/posts/practical-nix-flake-anatomy-a-guided-tour-of-flake.nix/)
- [x] Use NixOps with flakes:
  - [Using NixOS on your self-hosted server](https://old.reddit.com/r/selfhosted/comments/1cx4cjg/using_nixos_on_your_selfhosted_server/)
  - [NixOps with flakes - Help - NixOS Discourse](https://discourse.nixos.org/t/nixops-with-flakes/13306/6)
  -  [Practical Nix flake anatomy: a guided tour of flake.nix | Vladimir Timofeenko's blog](https://vtimofeenko.com/posts/practical-nix-flake-anatomy-a-guided-tour-of-flake.nix/)
  -  [Flake does not provide attribute - Help - NixOS Discourse](https://discourse.nixos.org/t/flake-does-not-provide-attribute/32156/2)