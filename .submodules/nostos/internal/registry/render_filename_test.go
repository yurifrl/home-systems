package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

// TestRenderTPINodeUsesNodeNameFilename pins the v0.2 fix for the
// empty-filename bug: when a tpi-managed node has no MAC, Render must
// write to <name>.yaml under state/configs/ — not "<empty>.yaml".
//
// Regression for nostos-v03 A4.
func TestRenderTPINodeUsesNodeNameFilename(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	const cfgYAML = `
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  schematic_id: 4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b
secrets:
  backend: env
nodes:
  tp1:
    ip: 192.168.68.107
    role: worker
    arch: arm64
    install_disk: /dev/mmcblk0
    template: tp1.yaml
    schematic_id: 3616c4c824f2540c0a14da0cc8e6fc46143f2ca0cc75c9c6376a66e562894950
    boot:
      method: tpi
      tpi:
        host: turingpi.local
        slot: 1
`
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "tp1.yaml"), []byte("machine:\n  type: worker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	p := paths.New(cfgPath)

	out, err := Render(cfg, p, "tp1", false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := filepath.Join(p.Configs(), "tp1.yaml")
	if out != want {
		t.Fatalf("Render path = %q, want %q (must NOT be \".yaml\" with empty MAC)", out, want)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("rendered file missing: %v", err)
	}
	// Negative: the empty-MAC bug would have written to "<configs>/.yaml".
	bad := filepath.Join(p.Configs(), ".yaml")
	if _, err := os.Stat(bad); err == nil {
		t.Fatalf("regression: empty-MAC bug resurfaced (%q exists)", bad)
	}
}
