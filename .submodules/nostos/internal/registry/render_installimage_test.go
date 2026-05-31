package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

// TestRenderResolvesInstallImage pins nostos-qfn.1: the Talos install image is
// rendered from config (talos_version + schematic) via the Go text/template
// pass, so templates use {{ .InstallImage }} instead of a hardcoded value.
func TestRenderResolvesInstallImage(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	const clusterSchematic = "4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b"
	const nodeSchematic = "3616c4c824f2540c0a14da0cc8e6fc46143f2ca0cc75c9c6376a66e562894950"
	const cfgYAML = `
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  schematic_id: ` + clusterSchematic + `
secrets:
  backend: env
nodes:
  dell01:
    mac: d0:94:66:d9:eb:a5
    ip: 192.168.68.101
    role: controlplane
    arch: amd64
    install_disk: /dev/nvme0n1
    template: dell01.yaml
  tp1:
    ip: 192.168.68.107
    role: worker
    arch: arm64
    install_disk: /dev/mmcblk0
    template: tp1.yaml
    schematic_id: ` + nodeSchematic + `
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
	const tmplBody = "machine:\n  install:\n    image: {{ .InstallImage }}\n"
	for _, f := range []string{"dell01.yaml", "tp1.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, "templates", f), []byte(tmplBody), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	p := paths.New(cfgPath)

	cases := []struct {
		node string
		want string
	}{
		// Cluster default schematic (no per-node override).
		{"dell01", "factory.talos.dev/metal-installer/" + clusterSchematic + ":v1.10.3"},
		// Per-node schematic override.
		{"tp1", "factory.talos.dev/metal-installer/" + nodeSchematic + ":v1.10.3"},
	}
	for _, tc := range cases {
		out, err := Render(cfg, p, tc.node, false)
		if err != nil {
			t.Fatalf("Render(%s): %v", tc.node, err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("read rendered %s: %v", tc.node, err)
		}
		if !strings.Contains(string(data), tc.want) {
			t.Fatalf("node %s rendered output missing %q; got:\n%s", tc.node, tc.want, string(data))
		}
	}
}
