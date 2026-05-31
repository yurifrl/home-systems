// Package paths resolves canonical file paths derived from a config.yaml location.
//
// Layout:
//
//	<config-dir>/
//	├── config.yaml
//	└── templates/
//
// Runtime cache lives outside the repo under the XDG user data directory:
//
//	~/.local/share/nostos/
//	├── assets/            # vmlinuz, initramfs, ipxe.efi, boot.ipxe
//	├── ipxe-src/          # iPXE source clone
//	├── configs/           # per-MAC rendered machineconfigs
//	├── cache/
//	├── logs/
//	└── pending-wipes.json
//
// Talos client files stay in the standard Talos directory:
//
//	~/.talos/config        # talosctl client config
//	~/.talos/kubeconfig    # kubeconfig fetched from Talos
package paths

import (
	"os"
	"path/filepath"
)

// Paths holds every canonical path for a consumer.
type Paths struct {
	Config string
}

// New wraps a config.yaml path.
func New(config string) Paths { return Paths{Config: config} }

func (p Paths) Root() string      { return filepath.Dir(p.Config) }
func (p Paths) Templates() string { return filepath.Join(p.Root(), "templates") }
func (p Paths) State() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "nostos")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "nostos")
	}
	return filepath.Join(p.Root(), "state")
}
func (p Paths) TalosDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".talos")
	}
	return filepath.Join(p.Root(), "state")
}
func (p Paths) Assets() string       { return filepath.Join(p.State(), "assets") }
func (p Paths) IpxeSrc() string      { return filepath.Join(p.State(), "ipxe-src") }
func (p Paths) Configs() string      { return filepath.Join(p.State(), "configs") }
func (p Paths) Talosconfig() string  { return filepath.Join(p.TalosDir(), "config") }
func (p Paths) Kubeconfig() string   { return filepath.Join(p.TalosDir(), "kubeconfig") }
func (p Paths) Cache() string        { return filepath.Join(p.State(), "cache") }
func (p Paths) Logs() string         { return filepath.Join(p.State(), "logs") }
func (p Paths) PendingWipes() string { return filepath.Join(p.State(), "pending-wipes.json") }

// EnsureState mkdirs every state subdirectory.
func (p Paths) EnsureState() error {
	for _, d := range []string{p.State(), p.Assets(), p.Configs(), p.Cache(), p.Logs()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
