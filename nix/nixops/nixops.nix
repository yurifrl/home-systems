{
  network = {
    description = "Legacy Network using <nixpkgs> and legacy state.";
    # NB this is not really what makes it a legacy network; lack of flakes is.
    storage.legacy = { };
  };
  server = { lib, pkgs, ... }: {
    imports = [ ../sd-image.nix ];

    deployment = {
      targetUser = "nixos";
      provisionSSHKey = true;
      targetEnv = "none";
      targetHost = "192.168.68.104";
    };

    environment.systemPackages = [ pkgs.hello pkgs.figlet ];
  };

  defaults = {
    users.extraUsers.nixos.openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPCfAwxYdoLR6YzoIx2+L593yLGpHaseGTCm3fxrshgD yurifl03@gmail.com"
    ];

  };
}
