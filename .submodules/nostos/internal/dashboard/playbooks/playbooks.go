// Package playbooks renders embedded vendor/model markdown playbooks via
// Glamour in strict-ANSI mode.
//
// v0.3 ships exactly two embedded playbooks: dell-optiplex-3080m and
// turing-rk1. Other vendors render a "create one with `nostos docs init`"
// placeholder.
package playbooks

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	glamour "charm.land/glamour/v2"
)

//go:embed embed/*.md
var content embed.FS

// List returns sorted IDs of embedded playbooks.
func List() []string {
	entries, err := fs.ReadDir(content, "embed")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(out)
	return out
}

// Raw returns the raw markdown for id, or ("", false) when absent.
func Raw(id string) (string, bool) {
	b, err := content.ReadFile("embed/" + id + ".md")
	if err != nil {
		return "", false
	}
	return string(b), true
}

// Render returns the strict-ANSI Glamour rendering for id, or a graceful
// placeholder when no playbook matches.
//
// Width is the terminal width to wrap to; 0 means use Glamour's default.
func Render(id string, width int) (string, error) {
	raw, ok := Raw(id)
	if !ok {
		return placeholder(id), nil
	}
	w := 100
	if width > 0 {
		w = width
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("notty"), // strict-ANSI: no colour, no raw HTML
		glamour.WithWordWrap(w),
	)
	if err != nil {
		return raw, err
	}
	out, err := r.Render(raw)
	if err != nil {
		return raw, err
	}
	return out, nil
}

func placeholder(id string) string {
	return fmt.Sprintf("no playbook for %q; create one with `nostos docs init %s`\n", id, id)
}
