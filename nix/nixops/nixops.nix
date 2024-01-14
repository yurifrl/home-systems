{
  network = {
    description = "Legacy Network using <nixpkgs> and legacy state.";
    # NB this is not really what makes it a legacy network; lack of flakes is.
    storage.legacy = {
      databasefile = "/nixops/deployments.nixops";
    };
  };
  # Machine
  master = { lib, pkgs, ... }: {
    imports = [
      ../sd-image.nix
      ./rpi4-hardware-configuration.nix
    ];
    deployment = {
      targetUser = "nixos";
      provisionSSHKey = true;
      targetEnv = "none";
      targetHost = "192.168.68.104";
    };
  };
  #
  defaults = { };
}
