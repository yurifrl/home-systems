package cluster

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

func TestFetchKubeconfigImportsTalosconfigFromProjectSource(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "nostos")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := paths.New(filepath.Join(configDir, "config.yaml"))

	xdg := filepath.Join(tmp, "xdg")
	t.Setenv("XDG_DATA_HOME", xdg)
	if err := os.MkdirAll(filepath.Dir(p.Talosconfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Talosconfig(), []byte("context: \"\"\ncontexts: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceDir := filepath.Join(tmp, "talos")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOSTOS_TEST_CA", "resolved-ca")
	source := "context: talos-default\ncontexts:\n  talos-default:\n    ca: env://NOSTOS_TEST_CA\n"
	if err := os.WriteFile(filepath.Join(sourceDir, "talosconfig"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeTalosctl := filepath.Join(binDir, "talosctl")
	script := `#!/bin/sh
if [ "$1" != "--talosconfig" ]; then
  echo "missing talosconfig flag" >&2
  exit 1
fi
if grep -q 'env://' "$2"; then
  echo "unresolved secret ref" >&2
  exit 1
fi
out=""
for arg in "$@"; do out="$arg"; done
printf 'kubeconfig\n' > "$out"
`
	if err := os.WriteFile(fakeTalosctl, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := &config.Config{Secrets: config.Secrets{Backend: "env"}}
	node := config.Node{IP: "192.0.2.10"}
	if err := FetchKubeconfig(context.Background(), cfg, p, node); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(p.Talosconfig())
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "context: talos-default") || !strings.Contains(got, "resolved-ca") {
		t.Fatalf("talosconfig was not imported and resolved:\n%s", got)
	}
	if strings.Contains(got, "env://") {
		t.Fatalf("talosconfig still contains unresolved env ref:\n%s", got)
	}
	if _, err := os.Stat(p.Kubeconfig()); err != nil {
		t.Fatalf("kubeconfig not written: %v", err)
	}
}
