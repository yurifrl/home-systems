{
  lib,
  buildGoModule,
  fetchFromGitHub,
}:

buildGoModule rec {
  pname = "raspberrypi-exporter";
  version = "master"; # Using master as there are no releases

  src = fetchFromGitHub {
    owner = "fahlke";
    repo = "raspberrypi_exporter";
    rev = "3417493c1f85c8470f2c85b4c9acb31ef473f0f4"; # Latest commit from master
    sha256 = "sha256-anb1h+nNtzitJsll5/8vJIYK6XRQk9LD6G/n/GkaDDM="; # Will need to be updated with correct hash
  };

  vendorHash = null; # Let Nix compute the vendor hash

  meta = with lib; {
    description = "Prometheus exporter for Raspberry Pi metrics";
    homepage = "https://github.com/fahlke/raspberrypi_exporter";
    license = licenses.mit;
    maintainers = [];
    platforms = platforms.linux;
  };
}
