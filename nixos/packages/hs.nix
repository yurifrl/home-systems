# nix-build -E '(import <nixpkgs> {}).callPackage ./hs.nix {}'  
{ lib, buildGoModule }:

buildGoModule rec {
  pname = "hs";
  version = "1.0.0"; # Update this with your version

  # Use the current directory as the source
  src = ../../.;

  vendorHash = "sha256-C2s52W2YqGiJZ6dqO9UflRSXmC85ODY7fRpWUSX83qY="; # If you are using vendored dependencies

  # Rename the binary otherwise it will be named after the package name
  installPhase = ''
    mkdir -p $out/bin
    cp -v $GOPATH/bin/* $out/bin/hs
  '';

  meta = with lib; {
    description = "Your description of the hs binary";
    homepage = "https://github.com/your-github-username/your-repo-name";
    license = licenses.mit; # Update this with your actual license
    maintainers = [ maintainers.your-username ]; # Update this with your username
  };
}
