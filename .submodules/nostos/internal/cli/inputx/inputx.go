// Package inputx hardens user-supplied CLI input.
//
// Every validator returns an *errs.Error with category=validation_failed.
package inputx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/yurifrl/nostos/internal/cli/errs"
)

var (
	nodeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)
	opRefRe    = regexp.MustCompile(`^op://[\w-]+/[\w.-]+(/[\w.-]+){0,2}$`)
	identRe    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// HasControl returns true if s contains any ASCII control char (0x00-0x1F + 0x7F)
// or any unicode control rune. ANSI ESC (0x1B) is included.
func HasControl(s string) bool {
	for _, r := range s {
		if r == 0x7F || (r < 0x20) || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// SanitizeForJSON strips control characters and ANSI CSI sequences from s.
// Used on any user input that gets echoed into structured output.
func SanitizeForJSON(s string) string {
	if s == "" {
		return s
	}
	// Strip ANSI CSI: ESC [ ... letter
	out := make([]rune, 0, len(s))
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == 0x1B && i+1 < len(runes) && runes[i+1] == '[' {
			// skip until letter
			j := i + 2
			for j < len(runes) {
				c := runes[j]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					break
				}
				j++
			}
			i = j
			continue
		}
		if r == 0x7F || (r < 0x20 && r != '\n' && r != '\t') {
			out = append(out, '?')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// ValidateNodeName enforces the AI-friendly node-name contract.
func ValidateNodeName(s string) error {
	if s == "" {
		return errs.Validation("E_NODE_NAME_EMPTY", "node name is empty").
			WithHint("provide a name from config.yaml: nostos node list")
	}
	if HasControl(s) {
		return errs.Validation("E_NODE_NAME_CONTROL", "node name contains control characters").
			WithHint("ASCII control characters (including ANSI escapes) are forbidden")
	}
	if len(s) > 63 {
		return errs.Validation("E_NODE_NAME_TOO_LONG",
			fmt.Sprintf("node name %q is %d chars (max 63)", SanitizeForJSON(s), len(s)))
	}
	if !nodeNameRe.MatchString(s) {
		return errs.Validation("E_NODE_NAME_FORMAT",
			fmt.Sprintf("node name %q must match [a-zA-Z0-9][a-zA-Z0-9-]{0,62}", SanitizeForJSON(s)))
	}
	return nil
}

// ValidateConfigPath rejects path traversal and absolute paths outside the
// operator home or current working directory tree.
func ValidateConfigPath(s string) error {
	if s == "" {
		return nil // empty is fine, caller will auto-discover
	}
	if HasControl(s) {
		return errs.Validation("E_CONFIG_PATH_CONTROL", "config path contains control characters")
	}
	clean := filepath.Clean(s)
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == ".." {
			return errs.Validation("E_CONFIG_PATH_TRAVERSAL",
				fmt.Sprintf("config path %q contains '..' segments", SanitizeForJSON(s)))
		}
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return errs.Validation("E_CONFIG_PATH_ABS", err.Error())
	}
	// If the path exists, resolve symlinks and require it lives under HOME or CWD.
	if _, statErr := os.Lstat(abs); statErr == nil {
		resolved, rErr := filepath.EvalSymlinks(abs)
		if rErr == nil {
			abs = resolved
		}
	}
	roots := allowedRoots()
	for _, root := range roots {
		if root == "" {
			continue
		}
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return nil
		}
	}
	return errs.Validation("E_CONFIG_PATH_OUTSIDE",
		fmt.Sprintf("config path %q resolves outside the operator home and CWD", SanitizeForJSON(s))).
		WithHint("place config under $HOME or the current working directory")
}

func allowedRoots() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, home)
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		out = append(out, cwd)
	}
	if tmp := os.TempDir(); tmp != "" {
		if resolved, err := filepath.EvalSymlinks(tmp); err == nil {
			out = append(out, resolved)
		}
		out = append(out, tmp)
	}
	return out
}

// ValidateOpRef ensures a 1Password reference is well-formed: op://vault/item[/section][/field].
func ValidateOpRef(s string) error {
	if HasControl(s) {
		return errs.Validation("E_OPREF_CONTROL", "op:// reference contains control characters")
	}
	if strings.ContainsAny(s, "?#") {
		return errs.Validation("E_OPREF_QUERY",
			fmt.Sprintf("op:// reference %q must not contain query parameters or fragments", SanitizeForJSON(s)))
	}
	if !opRefRe.MatchString(s) {
		return errs.Validation("E_OPREF_FORMAT",
			fmt.Sprintf("op:// reference %q does not match op://vault/item[/section][/field]", SanitizeForJSON(s)))
	}
	return nil
}

// ValidateFieldsMask checks every comma-separated field is in schema.
// Returns the parsed list (preserving order) on success.
func ValidateFieldsMask(s string, schema []string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	if HasControl(s) {
		return nil, errs.Validation("E_FIELDS_CONTROL", "fields mask contains control characters")
	}
	allowed := map[string]bool{}
	for _, f := range schema {
		allowed[f] = true
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, errs.Validation("E_FIELDS_EMPTY", "fields mask has empty entry")
		}
		if !identRe.MatchString(p) {
			return nil, errs.Validation("E_FIELDS_IDENT",
				fmt.Sprintf("field %q is not a valid identifier", SanitizeForJSON(p)))
		}
		if !allowed[p] {
			return nil, errs.Validation("E_FIELDS_UNKNOWN",
				fmt.Sprintf("unknown field %q (allowed: %s)", SanitizeForJSON(p), strings.Join(schema, ","))).
				WithDetails(map[string]any{"field": p, "allowed": schema})
		}
		out = append(out, p)
	}
	return out, nil
}
