package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBackend holds a canned vault — never touches real secrets.
type fakeBackend struct{ vault map[string]string }

func (fakeBackend) Scheme() string   { return "op" }
func (fakeBackend) Validate() error  { return nil }
func (f fakeBackend) Resolve(u string) (string, error) {
	if v, ok := f.vault[u]; ok {
		return v, nil
	}
	return "", &ResolveError{URI: u, Reason: "not in fake vault"}
}

func TestFindURIsMatchesRegistered(t *testing.T) {
	known := map[string]Backend{"op": fakeBackend{}, "env": EnvBackend{}, "file": FileBackend{}}
	text := "TS_AUTHKEY=op://kubernetes/talos/TS_AUTHKEY\nurl: https://example.com\nCA=env://MY_CA\nRAW=file:///tmp/x\n"
	uris := FindURIs(text, known)
	want := map[string]bool{
		"op://kubernetes/talos/TS_AUTHKEY": false,
		"env://MY_CA":                      false,
		"file:///tmp/x":                    false,
	}
	for _, u := range uris {
		want[u] = true
	}
	for u, got := range want {
		if !got {
			t.Errorf("missing URI %q in find output %v", u, uris)
		}
	}
	for _, u := range uris {
		if strings.HasPrefix(u, "https://") {
			t.Errorf("https:// should not be matched: %s", u)
		}
	}
}

func TestResolveTemplate(t *testing.T) {
	be := map[string]Backend{
		"op": fakeBackend{vault: map[string]string{
			"op://kubernetes/talos/TS_AUTHKEY": "tskey-fake",
		}},
	}
	got, err := ResolveTemplate("X=op://kubernetes/talos/TS_AUTHKEY\n", be)
	if err != nil {
		t.Fatal(err)
	}
	if got != "X=tskey-fake\n" {
		t.Errorf("got %q", got)
	}
}

func TestResolveTemplatePreservesHTTPS(t *testing.T) {
	be := map[string]Backend{"op": fakeBackend{}}
	text := "endpoint: https://192.168.68.100:6443\n"
	got, _ := ResolveTemplate(text, be)
	if got != text {
		t.Errorf("https pass-through broken: %q", got)
	}
}

func TestResolveTemplateErrorDoesNotLeakValue(t *testing.T) {
	be := map[string]Backend{"op": fakeBackend{vault: map[string]string{
		"op://a/b/c": "super-secret",
	}}}
	_, err := ResolveTemplate("X=op://missing/x/y\n", be)
	if err == nil || !strings.Contains(err.Error(), "op://missing/x/y") {
		t.Fatalf("want URI in err, got %v", err)
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Errorf("secret leaked into error: %v", err)
	}
}

func TestEnvBackend(t *testing.T) {
	t.Setenv("NOSTOS_TEST_S", "hunter2")
	got, err := EnvBackend{}.Resolve("env://NOSTOS_TEST_S")
	if err != nil || got != "hunter2" {
		t.Errorf("got %q err %v", got, err)
	}
}

func TestEnvBackendMissing(t *testing.T) {
	os.Unsetenv("NOSTOS_TEST_MISSING")
	_, err := EnvBackend{}.Resolve("env://NOSTOS_TEST_MISSING")
	if err == nil || !strings.Contains(err.Error(), "is not set") {
		t.Fatalf("want missing var err, got %v", err)
	}
}

func TestEnvBackendEmptyName(t *testing.T) {
	_, err := EnvBackend{}.Resolve("env://")
	if err == nil || !strings.Contains(err.Error(), "empty variable name") {
		t.Fatalf("want empty-name err, got %v", err)
	}
}

func TestFileBackend(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(p, []byte("hello\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := FileBackend{}.Resolve("file://" + p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("got %q want hello", got)
	}
}

func TestFileBackendMissing(t *testing.T) {
	_, err := FileBackend{}.Resolve("file:///definitely/does/not/exist")
	if err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("want read error, got %v", err)
	}
}
