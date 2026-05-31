package upgrade

import "testing"

func TestParseServerVersionShort(t *testing.T) {
	out := "Client:\n\tTag: v1.10.5\nServer:\n\tNODE: 192.168.1.10\n\tTag: v1.10.3\n"
	v, err := parseServerVersion(out)
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{1, 10, 3}) {
		t.Errorf("got %v, want v1.10.3", v)
	}
}

func TestParseServerVersionSingleLine(t *testing.T) {
	out := "Client: v1.10.5\nServer: v1.12.0\n"
	v, err := parseServerVersion(out)
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{1, 12, 0}) {
		t.Errorf("got %v, want v1.12.0", v)
	}
}

func TestParseServerVersionMissing(t *testing.T) {
	if _, err := parseServerVersion("Client:\n\tTag: v1.10.5\n"); err == nil {
		t.Fatal("expected error when no server version present")
	}
}

func TestInstallerImage(t *testing.T) {
	got := InstallerImage("abc123", Version{1, 13, 3})
	want := "factory.talos.dev/installer/abc123:v1.13.3"
	if got != want {
		t.Errorf("InstallerImage = %q, want %q", got, want)
	}
}
