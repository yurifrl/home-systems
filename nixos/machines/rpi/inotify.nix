{ pkgs, ... }:
{
#   # Increase system-wide file descriptor limits
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

#   # Increase the system-wide file watch limit
#   boot.kernel.sysctl = {
#     "fs.file-max" = 2097152;
#     "fs.inotify.max_user_watches" = 524288;
#     "fs.inotify.max_user_instances" = 512;
#   };
}