terraform {
  backend "remote" {
    workspaces {
      name = "home-systems"
    }
  }
}


module "nixos_image" {
  source  = "git::https://github.com/tweag/terraform-nixos.git//aws_image_nixos?ref=5f5a0408b299874d6a29d1271e9bffeee4c9ca71"
  release = "20.09"
}
