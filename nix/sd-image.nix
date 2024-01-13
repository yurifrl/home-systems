# save as sd-image.nix
{ lib, pkgs, ... }: {
  imports = [
    # Import SD card installer modules
    <nixpkgs/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix>
    # For nixpkgs cache
    <nixpkgs/nixos/modules/installer/cd-dvd/channel.nix>
  ];

  # If true, will build a .zst compressed image.
  sdImage.compressImage = false;
  # sdImage.enable = true; # What does this do?

  system.stateVersion = "23.05";

  # Define system packages
  environment.systemPackages = with pkgs; [
    vim
    curl
    htop
    cowsay
    hello
    fortune
    # Other packages can be uncommented as needed
  ];

  # Networking setup
  networking.useDHCP = false;
  networking.interfaces.eth0.useDHCP = true;
  networking.interfaces.wlan0.useDHCP = true;

  # SSH configuration
  services.openssh.enable = true;
  services.openssh.settings.PermitRootLogin = "yes";
  users.extraUsers.nixos.openssh.authorizedKeys.keys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPCfAwxYdoLR6YzoIx2+L593yLGpHaseGTCm3fxrshgD yurifl03@gmail.com"
  ];

  # OpenSSH is forced to have an empty `wantedBy` on the installer system[1], this won't allow it
  # to be automatically started. Override it with the normal value.
  # [1] https://github.com/NixOS/nixpkgs/blob/9e5aa25/nixos/modules/profiles/installation-device.nix#L76
  systemd.services.sshd.wantedBy = lib.mkOverride 40 [ "multi-user.target" ];

  # Enable OpenSSH out of the box.
  services.sshd.enable = true;

  # NTP time sync.
  services.timesyncd.enable = true;
}
