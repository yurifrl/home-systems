{pkgs, lib, ...}:

let
  version = "1.0.2";
  
  # Map of system to binary info
  binaries = {
    "x86_64-linux" = {
      name = "sbc_exporter-linux-amd64";
      sha256 = ""; # TODO: Add sha256 after first build attempt
    };
    "aarch64-linux" = {
      name = "sbc_exporter-linux-arm64";
      sha256 = ""; # TODO: Add sha256 after first build attempt
    };
    "x86_64-darwin" = {
      name = "sbc_exporter-darwin-amd64";
      sha256 = ""; # TODO: Add sha256 after first build attempt
    };
    "aarch64-darwin" = {
      name = "sbc_exporter-darwin-arm64"; 
      sha256 = ""; # TODO: Add sha256 after first build attempt
    };
  };

  binary = binaries.${pkgs.stdenv.hostPlatform.system} or (throw "Unsupported system: ${pkgs.stdenv.hostPlatform.system}");

in pkgs.stdenv.mkDerivation {
  pname = "sbc-exporter";
  inherit version;

  src = pkgs.fetchurl {
    url = "https://github.com/yurifrl/sbc_exporter/releases/download/v${version}/${binary.name}";
    sha256 = binary.sha256;
  };

  dontUnpack = true;

  installPhase = ''
    mkdir -p $out/bin
    cp $src $out/bin/sbc-exporter
    chmod +x $out/bin/sbc-exporter
  '';

  meta = with lib; {
    description = "SBC Exporter for Prometheus";
    homepage = "https://github.com/yurifrl/sbc_exporter";
    license = licenses.mit;
    platforms = builtins.attrNames binaries;
    maintainers = with maintainers; [ yurifrl ];
  };
}
