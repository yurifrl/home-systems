{
  lib,
  stdenv,
  fetchFromGitHub,
  makeWrapper,
  vcgencmd,
  metricsDir ? "/var/lib/raspberrypi-exporter",
}:

stdenv.mkDerivation rec {
  pname = "raspberrypi-exporter";
  version = "master";

  src = fetchFromGitHub {
    owner = "fahlke";
    repo = "raspberrypi_exporter";
    rev = "3417493c1f85c8470f2c85b4c9acb31ef473f0f4";
    sha256 = "sha256-anb1h+nNtzitJsll5/8vJIYK6XRQk9LD6G/n/GkaDDM=";
  };

  nativeBuildInputs = [ makeWrapper ];

  installPhase = ''
    install -Dm755 raspberrypi_exporter $out/bin/raspberrypi_exporter
    mkdir -p ${metricsDir}
    substituteInPlace $out/bin/raspberrypi_exporter \
      --replace '/var/lib/node_exporter/textfile_collector' '${metricsDir}'
    wrapProgram $out/bin/raspberrypi_exporter \
      --prefix PATH : ${lib.makeBinPath [ vcgencmd ]}
  '';

  meta = with lib; {
    description = "Prometheus exporter for Raspberry Pi metrics";
    homepage = "https://github.com/fahlke/raspberrypi_exporter";
    license = licenses.mit;
    maintainers = [];
    platforms = platforms.linux;
  };
}
