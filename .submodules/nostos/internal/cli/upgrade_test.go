package cli_test

import (
	"errors"
	"testing"

	"github.com/yurifrl/nostos/internal/cli/errs"
)

// Invalid --to version yields a validation error (exit 10) before any network.
func TestUpgradeInvalidVersion(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "upgrade", "--to", "bogus", "--dry-run")
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed error, got %T", err)
	}
	if typed.Category != errs.CatValidation || typed.Exit() != 10 {
		t.Errorf("wrong category/exit: %v %d", typed.Category, typed.Exit())
	}
}

// NODE plus explicit --all is a conflict (exit 10), caught before network.
func TestUpgradeNodeAllConflict(t *testing.T) {
	cfg := writeTestConfig(t)
	_, _, err := run(t, "--config", cfg, "--output", "json", "upgrade", "test1", "--all", "--to", "v1.13.3")
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
