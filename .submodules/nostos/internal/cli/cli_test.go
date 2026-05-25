package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/yurifrl/nostos/internal/cli"
	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/schema"
	"github.com/yurifrl/nostos/internal/mcp"
)

// run executes the cobra root with argv, capturing stdout + stderr.
func run(t *testing.T, argv ...string) (stdout, stderr string, err error) {
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

func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `cluster:
  name: test
  endpoint: https://192.168.99.99:6443
  talos_version: v1.10.3
  schematic_id: 6e4e8b75e7c1cf2c3f1cf2c3f1cf2c3f1cf2c3f1cf2c3f1cf2c3f1cf2c3f1cf2
secrets:
  backend: onepassword
  onepassword:
    account: foo.1password.com
    vault: kubernetes
nodes:
  test1:
    mac: "aa:bb:cc:dd:ee:ff"
    ip: 192.168.99.99
    role: controlplane
    arch: amd64
    install_disk: /dev/nvme0n1
    template: dell01.yaml
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	// templates dir + dell01 stub so render tests don't blow up if exercised
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "dell01.yaml"), []byte("version: v1alpha1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// C2: schema commands round-trip.
func TestSchemaAll(t *testing.T) {
	stdout, _, err := run(t, "schema", "--all")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]schema.Method
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(m) < 20 {
		t.Errorf("expected many methods, got %d", len(m))
	}
	if _, ok := m["node.install"]; !ok {
		t.Error("node.install missing from schema")
	}
}

func TestSchemaMethodIncludesAllFlags(t *testing.T) {
	stdout, _, err := run(t, "schema", "node.install")
	if err != nil {
		t.Fatal(err)
	}
	var m schema.Method
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"reinstall", "yes", "dry-run", "output"}
	got := map[string]bool{}
	for _, f := range m.Flags {
		got[f.Name] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("expected flag %q in schema, got %v", w, got)
		}
	}
	if !m.Destructive {
		t.Error("node.install must be destructive")
	}
}

// C5: structured error JSON shape + exit code.
func TestStructuredErrorShape(t *testing.T) {
	// Run a command that fails validation deterministically.
	cfg := writeTestConfig(t)
	stdout, _, err := run(t, "--config", cfg, "--output", "json", "node", "show", "no-such")
	if err == nil {
		t.Fatal("expected error")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if typed.Category != errs.CatNotFound {
		t.Errorf("wrong category: %v", typed.Category)
	}
	if typed.Exit() != 14 {
		t.Errorf("wrong exit code: %d", typed.Exit())
	}
	_ = stdout // stdout written by HandleExit, not Execute itself
}

// C3: --fields projection.
func TestFieldsMaskProjection(t *testing.T) {
	cfg := writeTestConfig(t)
	stdout, _, err := run(t, "--config", cfg, "--output", "json", "node", "list", "--fields", "name,ip")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid NDJSON line %q: %v", line, err)
		}
		if len(m) != 2 {
			t.Errorf("expected exactly 2 keys, got %v", m)
		}
		if _, ok := m["name"]; !ok {
			t.Errorf("missing name in %v", m)
		}
	}
}

func TestFieldsMaskUnknown(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "node", "list", "--fields", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Category != errs.CatValidation || typed.Exit() != 10 {
		t.Errorf("wrong category/exit: %v %d", typed.Category, typed.Exit())
	}
}

// C4: --dry-run emits canonical Plan and never spawns subprocesses.
// (We can't directly assert "no subprocess" here without a FakeCommander, but
// the dry-run code path returns before any external call.)
func TestNodeInstallDryRun(t *testing.T) {
	cfg := writeTestConfig(t)
	stdout, _, err := run(t, "--config", cfg, "--output", "json", "node", "install", "test1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Status       string           `json:"status"`
		WouldExecute []map[string]any `json:"would_execute"`
	}
	if err := json.Unmarshal([]byte(stdout), &p); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if p.Status != "preview" {
		t.Errorf("status=%q want preview", p.Status)
	}
	if len(p.WouldExecute) == 0 {
		t.Error("would_execute is empty")
	}
}

func TestRenderDryRun(t *testing.T) {
	cfg := writeTestConfig(t)
	stdout, _, err := run(t, "--config", cfg, "--output", "json", "render", "test1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"status": "preview"`) {
		t.Errorf("missing status: %s", stdout)
	}
	if !strings.Contains(stdout, `"would_execute"`) {
		t.Errorf("missing would_execute: %s", stdout)
	}
}

func TestSecretsKeysRevokeDryRun(t *testing.T) {
	cfg := writeTestConfig(t)
	stdout, _, err := run(t, "--config", cfg, "--output", "json", "secrets", "keys", "revoke", "kabc", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"status": "preview"`) {
		t.Errorf("missing preview: %s", stdout)
	}
}

// C2 round-trip: every cobra leaf has a schema entry containing every flag.
func TestSchemaCoversEveryFlag(t *testing.T) {
	root := cli.NewRoot("test")
	all := schema.All(root)
	for id, m := range all {
		cur := root
		ok := true
		for _, p := range strings.Split(id, ".") {
			matched := false
			for _, c := range cur.Commands() {
				if c.Name() == p {
					cur = c
					matched = true
					break
				}
			}
			if !matched {
				ok = false
				break
			}
		}
		if !ok {
			t.Fatalf("schema id %s not in cobra tree", id)
		}
		schemaFlags := map[string]bool{}
		for _, f := range m.Flags {
			schemaFlags[f.Name] = true
		}
		cur.Flags().VisitAll(func(f *pflag.Flag) {
			if !schemaFlags[f.Name] {
				t.Errorf("method %s missing flag %s in schema", id, f.Name)
			}
		})
	}
}

// C8: MCP tools/list returns one tool per cobra command.
func TestMCPToolsList(t *testing.T) {
	root := cli.NewRoot("test")
	srv := mcp.NewServer(root)
	in := strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","id":1}` + "\n")
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	if err := srv.Serve(context.Background(), in, out, errBuf); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if resp.Error != nil {
		t.Fatalf("rpc error: %v", resp.Error)
	}
	if len(resp.Result.Tools) < 20 {
		t.Errorf("expected many tools, got %d", len(resp.Result.Tools))
	}
}

func TestMCPSchemaSourceOfTruth(t *testing.T) {
	// Same number of tools as schema methods.
	root := cli.NewRoot("test")
	all := schema.All(root)
	srv := mcp.NewServer(root)
	tools := srv.Tools()
	if len(tools) != len(all) {
		t.Errorf("tools=%d schema=%d", len(tools), len(all))
	}
}
