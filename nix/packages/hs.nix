{ lib, stdenv, buildGoModule }:

buildGoModule rec {
  pname = "hs";
  version = "1.0.0"; # Update this with your version

  # Use the current directory as the source
  src = ./.;

  vendorSha256 = null; # If you are using vendored dependencies

  meta = with lib; {
    description = "Your description of the hs binary";
    homepage = "https://github.com/yurifrl/hs";
    license = licenses.mit; # Update this with your actual license
    maintainers = [ "yurifrl" ]; # Update this with your username
  };
}

