package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("cluster: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitWins(t *testing.T) {
	t.Setenv("NOSTOS_CONFIG", "")
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	touch(t, a)
	touch(t, filepath.Join(dir, "config.yaml"))

	got, err := FindConfig(a, dir)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(a)
	if got != abs {
		t.Errorf("got %q want %q", got, abs)
	}
}

func TestExplicitMissing(t *testing.T) {
	t.Setenv("NOSTOS_CONFIG", "")
	dir := t.TempDir()
	_, err := FindConfig(filepath.Join(dir, "nope.yaml"), dir)
	if err == nil || !strings.Contains(err.Error(), "--config") {
		t.Fatalf("want --config error, got %v", err)
	}
}

func TestEnvBeatsCwd(t *testing.T) {
	dir := t.TempDir()
	envCfg := filepath.Join(dir, "env.yaml")
	touch(t, envCfg)
	touch(t, filepath.Join(dir, "config.yaml"))
	t.Setenv("NOSTOS_CONFIG", envCfg)

	got, _ := FindConfig("", dir)
	absEnv, _ := filepath.Abs(envCfg)
	if got != absEnv {
		t.Errorf("got %q want %q", got, absEnv)
	}
}

func TestEnvMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOSTOS_CONFIG", filepath.Join(dir, "missing.yaml"))
	_, err := FindConfig("", dir)
	if err == nil || !strings.Contains(err.Error(), "NOSTOS_CONFIG") {
		t.Fatalf("want NOSTOS_CONFIG error, got %v", err)
	}
}

func TestCwdDiscovered(t *testing.T) {
	t.Setenv("NOSTOS_CONFIG", "")
	dir := t.TempDir()
	cwdCfg := filepath.Join(dir, "config.yaml")
	touch(t, cwdCfg)

	got, _ := FindConfig("", dir)
	abs, _ := filepath.Abs(cwdCfg)
	if got != abs {
		t.Errorf("got %q want %q", got, abs)
	}
}

func TestWalksUp(t *testing.T) {
	t.Setenv("NOSTOS_CONFIG", "")
	root := t.TempDir()
	nostosCfg := filepath.Join(root, "nostos", "config.yaml")
	touch(t, nostosCfg)

	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ := FindConfig("", sub)
	abs, _ := filepath.Abs(nostosCfg)
	if got != abs {
		t.Errorf("got %q want %q", got, abs)
	}
}

func TestCwdBeatsWalk(t *testing.T) {
	t.Setenv("NOSTOS_CONFIG", "")
	root := t.TempDir()
	touch(t, filepath.Join(root, "config.yaml"))
	touch(t, filepath.Join(root, "nostos", "config.yaml"))

	got, _ := FindConfig("", root)
	wantAbs, _ := filepath.Abs(filepath.Join(root, "config.yaml"))
	if got != wantAbs {
		t.Errorf("got %q want %q", got, wantAbs)
	}
}

func TestNoneFound(t *testing.T) {
	t.Setenv("NOSTOS_CONFIG", "")
	dir := t.TempDir()
	_, err := FindConfig("", dir)
	if err == nil || !strings.Contains(err.Error(), "no config.yaml") {
		t.Fatalf("want no config error, got %v", err)
	}
}
