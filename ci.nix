{ pkgs, ... }:
{
  herculesCI = {
    # Define systems to build for
    ciSystems = [
      "x86_64-linux"
      "aarch64-linux"
    ];

    # On push handler
    onPush.default = {
      outputs = { ... }: {
        # NixOS configurations
        nixos.rpi = self.nixosConfigurations.rpi.config.system.build.toplevel;
        nixos.vm = self.nixosConfigurations.vm.config.system.build.toplevel;

        # Raspberry Pi image
        packages.pi-image = self.packages.aarch64-linux.pi-image;

        # Run all flake checks
        inherit (self) checks;
      };
    };

    # Scheduled jobs
    onSchedule.nightly = {
      # Run every day at 2 AM UTC
      when = {
        hour = 2;
        minute = 0;
      };
      
      outputs = { ... }: {
        # Build everything nightly to ensure continued compatibility
        nixos.rpi = self.nixosConfigurations.rpi.config.system.build.toplevel;
        nixos.vm = self.nixosConfigurations.vm.config.system.build.toplevel;
        packages.pi-image = self.packages.aarch64-linux.pi-image;
      };
    };
  };
} 