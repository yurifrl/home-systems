Here is the content for `configuration.md`:

```markdown
# NixOS Configuration

## Example Dockerfile
This example Dockerfile demonstrates how to build NixOS in a container:
```Dockerfile
COPY nix/ .

RUN nix \
    --option filter-syscalls false \
    --show-trace \
    build

RUN mv result /result
```

## Configuration Editors
For editing NixOS configurations, refer to the [NixOS configuration editors](https://nixos.wiki/wiki/NixOS_configuration_editors).

## Nix Packages Discovery
```nix
❯ nix repl
nix-repl> :l . # In a flake
nix-repl> fooConfigurations.default.network.storage.legacy # Then you can look at stuff
```

## Terraform Info
For additional configuration and deployment automation, you can use Terraform. Refer to the resources in the references for more detailed guidance on using Terraform with NixOS.
