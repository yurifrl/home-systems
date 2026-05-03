// Package paths resolves canonical file paths derived from a config.yaml location.
//
// Layout:
//   <config-dir>/
//   ├── config.yaml
//   ├── templates/
//   └── state/
//       ├── assets/            # vmlinuz, initramfs, ipxe.efi, boot.ipxe
//       ├── ipxe-src/          # iPXE source clone
//       ├── configs/           # per-MAC rendered machineconfigs
//       ├── talosconfig
//       ├── kubeconfig
//       ├── cache/
//       ├── logs/
//       └── pending-wipes.json
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

func (p Paths) Root() string          { return filepath.Dir(p.Config) }
func (p Paths) Templates() string     { return filepath.Join(p.Root(), "templates") }
func (p Paths) State() string         { return filepath.Join(p.Root(), "state") }
func (p Paths) Assets() string        { return filepath.Join(p.State(), "assets") }
func (p Paths) IpxeSrc() string       { return filepath.Join(p.State(), "ipxe-src") }
func (p Paths) Configs() string       { return filepath.Join(p.State(), "configs") }
func (p Paths) Talosconfig() string   { return filepath.Join(p.State(), "talosconfig") }
func (p Paths) Kubeconfig() string    { return filepath.Join(p.State(), "kubeconfig") }
func (p Paths) Cache() string         { return filepath.Join(p.State(), "cache") }
func (p Paths) Logs() string          { return filepath.Join(p.State(), "logs") }
func (p Paths) PendingWipes() string  { return filepath.Join(p.State(), "pending-wipes.json") }

// EnsureState mkdirs every state subdirectory.
func (p Paths) EnsureState() error {
	for _, d := range []string{p.State(), p.Assets(), p.Configs(), p.Cache(), p.Logs()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
