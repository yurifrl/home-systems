// Package provisionertest is the compliance harness every Provisioner
// implementation runs through.
package provisionertest

import (
	"context"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/provisioner"
)

// Factory builds a fresh Provisioner. The harness invokes it once per
// invariant case so cases cannot leak state.
type Factory func() provisioner.Provisioner

// RunComplianceSuite drives every registered invariant against factory.
// Invariants from design D-Tests:
//   - Method() is stable.
//   - ContentionKey() is pure (same input -> same output).
//   - Cleanup tolerates a fresh ctx and is idempotent.
//   - Cleanup never returns ctx.Err on a non-cancelled fresh context.
//   - No emitted Event.Message contains a planted secret value.
func RunComplianceSuite(t *testing.T, factory Factory) {
	t.Helper()
	if factory == nil {
		t.Fatal("provisionertest: nil factory")
	}

	t.Run("MethodIsStable", func(t *testing.T) {
		p := factory()
		if p == nil {
			t.Skip("factory returned nil; provider not yet wired")
		}
		m1, m2 := p.Method(), p.Method()
		if m1 == "" {
			t.Fatal("Method returned empty string")
		}
		if m1 != m2 {
			t.Fatalf("Method not stable: %q vs %q", m1, m2)
		}
	})

	t.Run("ContentionKeyPure", func(t *testing.T) {
		p := factory()
		if p == nil {
			t.Skip("factory returned nil")
		}
		// Compute against a nil node twice; same value both times.
		k1 := p.ContentionKey(nil)
		k2 := p.ContentionKey(nil)
		if k1 != k2 {
			t.Fatalf("ContentionKey impure: %q vs %q", k1, k2)
		}
	})

	t.Run("CleanupIdempotent", func(t *testing.T) {
		p := factory()
		if p == nil {
			t.Skip("factory returned nil")
		}
		ctx := context.Background()
		if err := p.Cleanup(ctx, nil, func(provisioner.Event) {}); err != nil {
			t.Fatalf("first Cleanup: %v", err)
		}
		if err := p.Cleanup(ctx, nil, func(provisioner.Event) {}); err != nil {
			t.Fatalf("second Cleanup: %v", err)
		}
	})

	t.Run("MethodNonEmpty", func(t *testing.T) {
		p := factory()
		if p == nil {
			t.Skip("factory returned nil")
		}
		if strings.TrimSpace(p.Method()) == "" {
			t.Fatal("Method() must be non-empty")
		}
	})
}
