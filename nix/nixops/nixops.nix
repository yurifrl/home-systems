{
  network = {
    description = "Home Systems";
    enableRollback = true;
    # NB this is not really what makes it a legacy network; lack of flakes is.
    storage.legacy = {
      databasefile = "/nixops/deployments.nixops";
    };
  };
  # Machine
  master = { lib, pkgs, ... }: {
    imports = [
      ../sd-image.nix
    ];
    deployment = {
      targetUser = "nixos";
      provisionSSHKey = true;
      targetEnv = "none";
      targetHost = "192.168.68.103";

      # # Will not use this secrets, will add this secret on creation
      # keys.test = {
      #   text = "hello";
      #   user = "tailscale";
      #   group = "keys";
      #   permissions = "0640";
      # };
    };
  };
  #
  defaults = { };
}
