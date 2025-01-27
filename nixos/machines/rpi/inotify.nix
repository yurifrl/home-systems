{ ... }:
{
  # System limits configuration
  security.pam.loginLimits = [
    {
      domain = "*";
      type = "soft";
      item = "nofile";
      value = "65535";
    }
    {
      domain = "*";
      type = "hard";
      item = "nofile";
      value = "65535";
    }
  ];

  # Increase system-wide file descriptor limit
  boot.kernel.sysctl = {
    "fs.file-max" = 65535;
    "fs.inotify.max_user_watches" = 65535;
    "fs.inotify.max_user_instances" = 8192;
  };

}