Here is the content for `cli.md`:

```markdown
# CLI Usage

The CLI needs to run in its Docker image to work properly.

```bash
docker compose run --rm hs
hs run
```

## Available Commands

### `hs nix build`
Build the NixOS configuration.

### `hs run`
Run the CLI in interactive mode.

### `hs new-sd`
Flash a new image to an SD card using Docker.

### Example Usage

To build the NixOS configuration:
```bash
hs nix build
```

To run the CLI interactively:
```bash
hs run
```

To flash a new SD card image:
```bash
hs new-sd
```

### Rework  Flash
- Example prompts during `hs flash`:
  - Build a new image or reuse an existing one.
  - Download from GitHub artifacts if available.
  - Provide options for image parameters and device selection.
- Note: The target device can be specified with `-fd`.
- Create a flash command that runs on any machine and in the end can either build or download a .img and flash to an sd or just return the .img or .iso
