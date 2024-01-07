with import <nixpkgs> { };

let
  pythonPackages = python3Packages;
in


{ pkgs ? import <nixpkgs> { } }:


pkgs.mkShell rec {
  name = "Cli";
  venvDir = "./.venv";
  buildInputs = [
    # A Python interpreter including the 'venv' module is required to bootstrap
    # the environment.
    pythonPackages.python

    # This executes some shell code to initialize a venv in $venvDir before
    # dropping into the shell
    pythonPackages.venvShellHook

    # In this particular example, in order to compile any binary extensions they may
    # require, the Python modules listed in the hypothetical requirements.txt need
    # the following packages to be installed locally:
    taglib
    openssl
    git
    libxml2
    libxslt
    libzip
    zlib
    #
    python3
    python3Packages.click
  ];

  # Run this command, only after creating the virtual environment
  postVenvCreation = ''
    unset SOURCE_DATE_EPOCH
    pip install -r requirements.txt
  '';

  # Now we can execute any commands within the virtual environment.
  # This is optional and can be left out to run pip manually.
  postShellHook = ''
    unset SOURCE_DATE_EPOCH
    # Make entrypoint.py executable
    chmod +x ./entrypoint.py
  '';

  # Add a custom shell function to run entrypoint.py
  shellHook = ''
    pip install -r requirements.txt
    function run() {
      python ./entrypoint.py "$@"
    }
  '';

}
