{ lib
, stdenv
, fetchFromGitHub
, curl
, unzip
, makeWrapper
}:

stdenv.mkDerivation rec {
  pname = "raspberrypi-exporter";
  version = "master";  # Repository doesn't use version tags

  src = fetchFromGitHub {
    owner = "fahlke";
    repo = "raspberrypi_exporter";
    rev = "master";
    sha256 = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";  # Will need to be updated
  };

  nativeBuildInputs = [ makeWrapper ];
  buildInputs = [ curl unzip ];

  installPhase = ''
    runHook preInstall

    # Create necessary directories
    mkdir -p $out/bin
    mkdir -p $out/lib/systemd/system

    # Install the main script
    install -Dm755 raspberrypi_exporter $out/bin/raspberrypi_exporter

    # Install systemd units
    install -Dm644 raspberrypi_exporter.service $out/lib/systemd/system/raspberrypi_exporter.service
    install -Dm644 raspberrypi_exporter.timer $out/lib/systemd/system/raspberrypi_exporter.timer

    # Wrap the script with required runtime dependencies
    wrapProgram $out/bin/raspberrypi_exporter \
      --prefix PATH : ${lib.makeBinPath [ curl ]}

    runHook postInstall
  '';

  meta = with lib; {
    description = "Prometheus exporter for Raspberry Pi metrics";
    homepage = "https://github.com/fahlke/raspberrypi_exporter";
    license = licenses.mit;
    platforms = platforms.linux;
    maintainers = [];
  };
} 