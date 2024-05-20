provider "local" {
  version = "~> 2.1"
}

module "deploy_nixos" {
  source               = "github.com/awakesecurity/terraform-nixos//deploy_nixos?ref=c4b1ee6d24b54e92fa3439a12bce349a6805bcdd"
  nixos_config         = "${path.module}/sd-image.nix"
  hermetic             = true
  target_user          = "root"
  target_host          = aws_instance.hermetic-nixos-system[0].public_ip
  ssh_private_key_file = pathexpand("~/.ssh/yourkeyname.pem")
}
