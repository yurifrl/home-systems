package cli_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/cli/errs"
)

// apply --dry-run emits a canonical Plan and spawns no subprocesses.
func TestApplyDryRun(t *testing.T) {
	cfg := writeTestConfig(t)
	stdout, _, err := run(t, "--config", cfg, "--output", "json", "apply", "test1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Status       string           `json:"status"`
		Method       string           `json:"method"`
		WouldExecute []map[string]any `json:"would_execute"`
	}
	if err := json.Unmarshal([]byte(stdout), &p); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if p.Status != "preview" {
		t.Errorf("status=%q want preview", p.Status)
	}
	if p.Method != "apply" {
		t.Errorf("method=%q want apply", p.Method)
	}
	if len(p.WouldExecute) == 0 {
		t.Error("would_execute is empty")
	}
}

// Unknown node yields E_NODE_NOT_FOUND (exit 14).
func TestApplyUnknownNode(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "apply", "no-such", "--mode", "no-reboot")
	if err == nil {
		t.Fatal("expected error")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Category != errs.CatNotFound || typed.Exit() != 14 {
		t.Errorf("wrong category/exit: %v %d", typed.Category, typed.Exit())
	}
}

// Invalid --mode yields a validation error (exit 10).
func TestApplyInvalidMode(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "apply", "test1", "--mode", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Category != errs.CatValidation || typed.Exit() != 10 {
		t.Errorf("wrong category/exit: %v %d", typed.Category, typed.Exit())
	}
}

// Reboot-capable mode without --yes is gated by confirmation (exit 13).
func TestApplyRebootRequiresConfirm(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "apply", "test1", "--mode", "reboot")
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Category != errs.CatConflict || typed.Exit() != 13 {
		t.Errorf("wrong category/exit: %v %d", typed.Category, typed.Exit())
	}
}

// auto mode (Talos decides; can reboot) also requires --yes.
func TestApplyAutoRequiresConfirm(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "apply", "test1")
	if err == nil {
		t.Fatal("expected confirmation error for default auto mode")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Exit() != 13 {
		t.Errorf("wrong exit: %d want 13", typed.Exit())
	}
}

// NODE and --all are mutually exclusive.
func TestApplyAllConflictsWithNode(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "apply", "test1", "--all", "--mode", "no-reboot")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Exit() != 10 {
		t.Errorf("wrong exit: %d want 10", typed.Exit())
	}
}

// No NODE and no --all is a validation error.
func TestApplyRequiresTarget(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "apply", "--mode", "no-reboot")
	if err == nil {
		t.Fatal("expected error when no target given")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Exit() != 10 {
		t.Errorf("wrong exit: %d want 10", typed.Exit())
	}
}

// apply --all --dry-run previews every node.
func TestApplyAllDryRun(t *testing.T) {
	cfg := writeTestConfig(t)
	stdout, _, err := run(t, "--config", cfg, "--output", "json", "apply", "--all", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"status": "preview"`) {
		t.Errorf("missing preview envelope:\n%s", stdout)
	}
}
