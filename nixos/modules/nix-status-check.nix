{ config, lib, pkgs, ... }:

let
  python-script = pkgs.writeText "simple-http-server.py" ''
    from http.server import HTTPServer, BaseHTTPRequestHandler

    class SimpleHandler(BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(200)
            self.send_header('Content-type', 'text/plain')
            self.end_headers()
            self.wfile.write(b'Hello World v1.0.0')

    server = HTTPServer(('0.0.0.0', 9876), SimpleHandler)
    print('Starting server on port 9876...')
    server.serve_forever()
  '';
in
{
  systemd.services.nix-status-check = {
    description = "Nix Status Check";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];

    serviceConfig = {
      ExecStart = "${pkgs.python3}/bin/python3 ${python-script}";
      Restart = "always";
      RestartSec = "10";
      User = "nobody";
    };
  };

  # networking.firewall.allowedTCPPorts = [ 9876 ];

  # # Only configure firewall if it's enabled
  # networking.firewall = lib.mkIf config.networking.firewall.enable {
  #   # Allow access from Tailscale network
  #   interfaces."tailscale0" = {
  #     allowedTCPPorts = [ 9876 ];  # Match the port above
  #   };
  # };
}
