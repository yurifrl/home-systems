{ pkgs, ... }:
{
  # Increase system-wide file descriptor limits
#   security.pam.loginLimits = [
#     {
#       domain = "*";
#       type = "soft";
#       item = "nofile";
#       value = "16384";
#     }
#     {
#       domain = "*";
#       type = "hard";
#       item = "nofile";
#       value = "524288";
#     }
#   ];
}