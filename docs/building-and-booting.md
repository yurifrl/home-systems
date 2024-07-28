Here is the content for `building-and-booting.md`:

```markdown
# Building and Booting the Image

## Building the Image
To build the NixOS image for an ARM device (e.g., Raspberry Pi), use the following command:
```bash
nix-build '<nixpkgs/nixos>' -A config.system.build.sdImage -I nixos-config=./sd-image.nix --argstr system aarch64-linux
```

After building, copy the image:
```bash
cp ./result/sd-image/*.img* .
```

## Searching for Packages
You can search for available Nix packages with:
```bash
nix search nixpkgs cowsay
```

## Inspecting the Current System
To inspect the current system, run:
```bash
nix eval --raw --impure --expr "builtins.currentSystem"
```
