package pxe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSudoersDropInContent(t *testing.T) {
	const (
		user     = "alice"
		dnsmasq  = "/opt/homebrew/sbin/dnsmasq"
		pkillBin = "/usr/bin/pkill"
	)
	got := SudoersDropInContent(user, dnsmasq, pkillBin)

	// Managed marker comment must be present.
	if !strings.Contains(got, sudoersMarker) {
		t.Errorf("content missing managed marker:\n%s", got)
	}
	// dnsmasq rule: user, path, and the wildcard for any args.
	dnsmasqLine := user + " ALL=(root) NOPASSWD: " + dnsmasq + " *"
	if !strings.Contains(got, dnsmasqLine) {
		t.Errorf("content missing dnsmasq NOPASSWD line %q:\n%s", dnsmasqLine, got)
	}
	// pkill rule scoped to dnsmasq.
	pkillLine := user + " ALL=(root) NOPASSWD: " + pkillBin + " -f dnsmasq*"
	if !strings.Contains(got, pkillLine) {
		t.Errorf("content missing pkill NOPASSWD line %q:\n%s", pkillLine, got)
	}
	// Must contain the user, dnsmasq path with wildcard, and the pkill rule.
	if !strings.Contains(got, user) {
		t.Errorf("content missing user %q", user)
	}
	if !strings.Contains(got, dnsmasq+" *") {
		t.Errorf("content missing dnsmasq path with wildcard")
	}
}

func TestSudoersDnsmasqLineMatchesContent(t *testing.T) {
	line := sudoersDnsmasqLine("bob", "/usr/local/sbin/dnsmasq")
	content := SudoersDropInContent("bob", "/usr/local/sbin/dnsmasq", "/usr/bin/pkill")
	if !strings.Contains(content, line) {
		t.Errorf("idempotency key line %q not found in content:\n%s", line, content)
	}
}

func TestDnsmasqBinaryNonEmpty(t *testing.T) {
	if got := dnsmasqBinary(); got == "" {
		t.Error("dnsmasqBinary() returned empty string")
	}
	if got := DnsmasqBinary(); got == "" {
		t.Error("DnsmasqBinary() returned empty string")
	}
}

func TestPkillBinaryNonEmpty(t *testing.T) {
	if got := pkillBinary(); got == "" {
		t.Error("pkillBinary() returned empty string")
	}
}

// sudoersInstalledAt is tested against a temp file so we never read real /etc
// and never require root.
func TestSudoersInstalledAt(t *testing.T) {
	dir := t.TempDir()
	marker := sudoersDnsmasqLine("carol", "/opt/homebrew/sbin/dnsmasq")

	// Missing file -> not installed.
	missing := filepath.Join(dir, "absent")
	if sudoersInstalledAt(missing, marker) {
		t.Error("expected false for missing file")
	}

	// File present with the marker line -> installed.
	present := filepath.Join(dir, "nostos-pxe")
	content := SudoersDropInContent("carol", "/opt/homebrew/sbin/dnsmasq", "/usr/bin/pkill")
	if err := os.WriteFile(present, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if !sudoersInstalledAt(present, marker) {
		t.Errorf("expected true when marker line present\ncontent:\n%s", content)
	}

	// File present but for a different dnsmasq path -> not installed.
	otherMarker := sudoersDnsmasqLine("carol", "/usr/sbin/dnsmasq")
	if sudoersInstalledAt(present, otherMarker) {
		t.Error("expected false when the dnsmasq path differs")
	}
}

func TestSudoersDropInPathConst(t *testing.T) {
	if SudoersDropInPath != "/etc/sudoers.d/nostos-pxe" {
		t.Errorf("unexpected drop-in path: %s", SudoersDropInPath)
	}
}
