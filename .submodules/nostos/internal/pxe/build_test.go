package pxe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

func setupPaths(t *testing.T) (paths.Paths, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  schematic_id: 4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b
secrets:
  backend: env
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	p := paths.New(cfgPath)
	if err := p.EnsureState(); err != nil {
		t.Fatal(err)
	}
	return p, cfg
}

func TestRenderBootIpxeUsesNextServer(t *testing.T) {
	p, cfg := setupPaths(t)
	out, err := RenderBootIpxe(cfg, p, "amd64", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(out)
	s := string(body)
	if !strings.Contains(s, "${next-server}") {
		t.Errorf("missing ${next-server}")
	}
	if strings.Contains(s, "192.168.") {
		t.Errorf("boot.ipxe hardcoded an IP:\n%s", s)
	}
	if !strings.Contains(s, "/assets/vmlinuz-amd64") {
		t.Errorf("wrong kernel path; output:\n%s", s)
	}
	if !strings.Contains(s, "/assets/initramfs-amd64.xz") {
		t.Errorf("wrong initramfs path; output:\n%s", s)
	}
	if !strings.Contains(s, "v1.10.3") {
		t.Errorf("Talos version missing")
	}
}

func TestRenderBootIpxeIncludesExtraArgs(t *testing.T) {
	p, cfg := setupPaths(t)
	out, err := RenderBootIpxe(cfg, p, "amd64", "talos.experimental.wipe=system")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(out)
	if !strings.Contains(string(body), "talos.experimental.wipe=system") {
		t.Errorf("wipe flag missing")
	}
}

func TestEmbedIpxeHasRetryLoop(t *testing.T) {
	for _, need := range []string{"retry_dhcp", "isset ${filename}", "chain ${filename}"} {
		if !strings.Contains(EmbedIpxe, need) {
			t.Errorf("EmbedIpxe missing %q", need)
		}
	}
}
