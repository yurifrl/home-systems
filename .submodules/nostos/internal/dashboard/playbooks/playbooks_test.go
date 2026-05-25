package playbooks

import (
	"strings"
	"testing"
)

func TestListIncludesShipped(t *testing.T) {
	got := List()
	want := map[string]bool{"dell-optiplex-3080m": true, "turing-rk1": true}
	for _, id := range got {
		delete(want, id)
	}
	if len(want) > 0 {
		t.Fatalf("missing embedded playbooks: %v", want)
	}
}

func TestRenderStability(t *testing.T) {
	a, err := Render("dell-optiplex-3080m", 80)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Render("dell-optiplex-3080m", 80)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("render is non-deterministic")
	}
	if !strings.Contains(a, "OptiPlex") {
		t.Fatalf("rendered output missing title content: %q", a)
	}
}

func TestRenderMissingPlaybook(t *testing.T) {
	out, err := Render("nope-9000", 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no playbook for") {
		t.Fatalf("placeholder missing: %q", out)
	}
}
