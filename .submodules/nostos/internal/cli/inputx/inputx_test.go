package inputx

import (
	"strings"
	"testing"
)

func TestValidateNodeName(t *testing.T) {
	good := []string{"tp1", "dell01", "node-1", "abc"}
	for _, g := range good {
		if err := ValidateNodeName(g); err != nil {
			t.Errorf("expected %q valid: %v", g, err)
		}
	}
	bad := []string{"", "-leading", "with space", "ctrl\x01x", "way" + strings.Repeat("a", 70), "a/b", "a\x1b[31mred"}
	for _, b := range bad {
		if err := ValidateNodeName(b); err == nil {
			t.Errorf("expected %q invalid", b)
		}
	}
}

func TestValidateOpRef(t *testing.T) {
	good := []string{"op://vault/item", "op://my-vault/item.name/section/field"}
	for _, g := range good {
		if err := ValidateOpRef(g); err != nil {
			t.Errorf("expected %q valid: %v", g, err)
		}
	}
	bad := []string{"http://x", "op://", "op://v/i?x=1", "op://v/i#frag", "op://v/i/s/f/extra", "op://v/i\x00"}
	for _, b := range bad {
		if err := ValidateOpRef(b); err == nil {
			t.Errorf("expected %q invalid", b)
		}
	}
}

func TestValidateFieldsMask(t *testing.T) {
	schema := []string{"name", "ip", "role"}
	got, err := ValidateFieldsMask("name,ip", schema)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "name" {
		t.Errorf("got %v", got)
	}
	if _, err := ValidateFieldsMask("bogus", schema); err == nil {
		t.Error("expected error for unknown field")
	}
	if _, err := ValidateFieldsMask("name,\x01", schema); err == nil {
		t.Error("expected error for control char")
	}
	if _, err := ValidateFieldsMask("name,", schema); err == nil {
		t.Error("expected error for empty entry")
	}
}

func TestValidateConfigPath(t *testing.T) {
	if err := ValidateConfigPath(""); err != nil {
		t.Errorf("empty should be allowed: %v", err)
	}
	if err := ValidateConfigPath("../../etc/passwd"); err == nil {
		t.Error("expected traversal rejection")
	}
	if err := ValidateConfigPath("rel/path.yaml"); err != nil {
		// Resolves under CWD; test runs in package dir, so this is allowed
		t.Errorf("relative under cwd should be ok: %v", err)
	}
	if err := ValidateConfigPath("/etc/passwd"); err == nil {
		t.Error("expected /etc/passwd outside roots")
	}
	if err := ValidateConfigPath("foo\x01.yaml"); err == nil {
		t.Error("expected control rejection")
	}
}

func TestSanitizeForJSON(t *testing.T) {
	in := "hello\x1b[31mred\x1b[0mworld\x07"
	out := SanitizeForJSON(in)
	if strings.ContainsRune(out, 0x1b) || strings.ContainsRune(out, 0x07) {
		t.Errorf("residual control chars: %q", out)
	}
}

func FuzzValidateNodeName(f *testing.F) {
	f.Add("tp1")
	f.Add("dell01")
	f.Add("node-1")
	f.Add("\x00\x01\x02")
	f.Add("a\x1b[31mb")
	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateNodeName(s)
		if err == nil {
			// must be ASCII, no control, length>0 && <=63
			if s == "" || len(s) > 63 {
				t.Fatalf("validator passed bad-length %q", s)
			}
			if HasControl(s) {
				t.Fatalf("validator passed control-char %q", s)
			}
		}
	})
}

func FuzzValidateOpRef(f *testing.F) {
	f.Add("op://vault/item")
	f.Add("op://v/i?x=1")
	f.Add("op://v/i#f")
	f.Add("not-a-ref")
	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateOpRef(s)
		if err == nil {
			if HasControl(s) || strings.ContainsAny(s, "?#") {
				t.Fatalf("validator passed bad opref %q", s)
			}
		}
	})
}

func FuzzValidateConfigPath(f *testing.F) {
	f.Add("config.yaml")
	f.Add("../../etc/passwd")
	f.Add("/etc/shadow")
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateConfigPath(s)
		if err == nil {
			if HasControl(s) {
				t.Fatalf("passed control in %q", s)
			}
			for _, p := range strings.Split(s, "/") {
				if p == ".." {
					t.Fatalf("passed traversal in %q", s)
				}
			}
		}
	})
}

func FuzzValidateFieldsMask(f *testing.F) {
	schema := []string{"name", "ip", "role"}
	f.Add("name,ip")
	f.Add("bogus")
	f.Add("name,\x01")
	f.Fuzz(func(t *testing.T, s string) {
		_, err := ValidateFieldsMask(s, schema)
		if err == nil && HasControl(s) {
			t.Fatalf("control accepted: %q", s)
		}
	})
}
