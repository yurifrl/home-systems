package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/cli"
)

// TestFlashDryRun emits a Plan envelope when --dry-run is set, with no
// network calls or file writes.
func TestFlashDryRun(t *testing.T) {
	cfg := writeFlashTestConfig(t)

	stdout, _, err := runFlash(t, "--config", cfg, "--output", "json", "flash", "test1", "--out", "/tmp/flash-test.raw", "--dry-run")
	if err != nil {
		t.Fatalf("flash --dry-run failed: %v\nstdout=%s", err, stdout)
	}

	var plan struct {
		Status        string `json:"status"`
		Method        string `json:"method"`
		WouldExecute  []struct {
			Phase  string `json:"phase"`
			Detail string `json:"detail"`
		} `json:"would_execute"`
	}
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if plan.Status != "preview" {
		t.Errorf("status: got %q, want %q", plan.Status, "preview")
	}
	if plan.Method != "flash" {
		t.Errorf("method: got %q, want %q", plan.Method, "flash")
	}
	mustHave := []string{"preflight", "download.image", "render", "assemble.file", "instructions"}
	got := map[string]bool{}
	for _, s := range plan.WouldExecute {
		got[s.Phase] = true
	}
	for _, p := range mustHave {
		if !got[p] {
			t.Errorf("dry-run missing phase %q; got phases=%v", p, got)
		}
	}
}

// TestFlashDryRunRPi adds the rpi_generic overlay and verifies the eeprom +
// rpi-firmware phases appear.
func TestFlashDryRunRPi(t *testing.T) {
	cfg := writeFlashTestConfigRPi(t)
	stdout, _, err := runFlash(t, "--config", cfg, "--output", "json", "flash", "rpi01", "--out", "/tmp/rpi.raw", "--dry-run")
	if err != nil {
		t.Fatalf("flash --dry-run rpi: %v\n%s", err, stdout)
	}
	for _, want := range []string{"download.rpi-firmware", "eeprom"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected phase %q in dry-run output, got:\n%s", want, stdout)
		}
	}
}

// TestFlashOutputRequired enforces --output OR --device.
func TestFlashOutputRequired(t *testing.T) {
	cfg := writeFlashTestConfig(t)
	_, _, err := runFlash(t, "--config", cfg, "--output", "json", "flash", "test1", "--dry-run")
	if err == nil {
		t.Fatal("expected error when neither --output nor --device is set")
	}
	if !strings.Contains(err.Error(), "either --out FILE or --device") {
		t.Errorf("wrong error: %v", err)
	}
}

// runFlash is a copy of run() so this file is self-contained even if cli_test.go
// shifts. (`run` is an unexported helper there.)
func runFlash(t *testing.T, argv ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := cli.NewRoot("test")
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs(argv)
	err = root.ExecuteContext(context.Background())
	return out.String(), errBuf.String(), err
}

func writeFlashTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `cluster:
  name: test
  endpoint: https://10.0.0.1:6443
  talos_version: v1.13.3
  schematic_id: 8f04ea6b6016f12a593fa8a87441270075c648cb75482c2d9d3db8cecda47da1
secrets:
  backend: env
nodes:
  test1:
    mac: "aa:bb:cc:dd:ee:ff"
    ip: 10.0.0.10
    role: controlplane
    arch: amd64
    install_disk: /dev/nvme0n1
    template: test1.yaml
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "test1.yaml"), []byte("version: v1alpha1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFlashTestConfigRPi(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `cluster:
  name: test
  endpoint: https://10.0.0.1:6443
  talos_version: v1.13.3
  schematic_id: 8f04ea6b6016f12a593fa8a87441270075c648cb75482c2d9d3db8cecda47da1
secrets:
  backend: env
nodes:
  rpi01:
    mac: "e4:5f:01:3c:68:fa"
    ip: 10.0.0.20
    role: controlplane
    arch: arm64
    install_disk: /dev/sda
    template: rpi01.yaml
    overlay: rpi_generic
    schematic_id: d0e797e79d4e8c53a843776a1a5b57a3429aaaf7e8e3246d35df5df9f915da86
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "rpi01.yaml"), []byte("version: v1alpha1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
