{
  network = {
    description = "Legacy Network using <nixpkgs> and legacy state.";
    # NB this is not really what makes it a legacy network; lack of flakes is.
    storage.legacy = { };
  };
  # Machine
  master = { lib, pkgs, ... }: {
    imports = [ ../sd-image.nix ];

    deployment = {
      targetUser = "nixos";
      provisionSSHKey = true;
      targetEnv = "none";
      targetHost = "192.168.68.104";
    };

    environment.systemPackages = [ pkgs.hello pkgs.figlet ];


    boot.loader.grub.enable = false;
    boot.loader.generic-extlinux-compatible.enable = true;


    boot.initrd.availableKernelModules = [ "usbhid" ];
    boot.initrd.kernelModules = [ ];
    boot.kernelModules = [ ];
    boot.extraModulePackages = [ ];
    boot.kernelPackages = pkgs.linuxPackages_rpi4;

    fileSystems."/" = {
      device = "/dev/disk/by-label/NIXOS_SD";
      fsType = "ext4";
    };

    swapDevices = [ ];
  };

  defaults = {

    # Enable openssh and add the ssh key to the root user
    services.openssh.enable = true;

    # Define that we need to build for ARM
    nixpkgs.localSystem = {
      system = "aarch64-linux";
      config = "aarch64-unknown-linux-gnu";
    };

    users.extraUsers.nixos.openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPCfAwxYdoLR6YzoIx2+L593yLGpHaseGTCm3fxrshgD yurifl03@gmail.com"
    ];

  };
}
