package pxe

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/yurifrl/nostos/internal/cli/errs"
)

// SudoersDropInPath is the canonical location of the nostos PXE sudoers
// drop-in. Installed by `nostos pxe setup`, consumed (sudo-less) by
// `nostos pxe` (serve), and inspected by the doctor command.
const SudoersDropInPath = "/etc/sudoers.d/nostos-pxe"

// sudoersMarker is the managed-comment header written at the top of the
// drop-in. SudoersInstalled() also keys idempotency off the dnsmasq rule line
// rather than this comment, so the comment is purely documentation.
const sudoersMarker = "# Managed by `nostos pxe setup` — scoped NOPASSWD for the PXE boot server."

// DnsmasqBinary is the exported accessor for the resolved dnsmasq path, used
// by the CLI to report which binary the drop-in was scoped to.
func DnsmasqBinary() string { return dnsmasqBinary() }

// dnsmasqBinary resolves the dnsmasq path the same way startDnsmasq does:
// prefer the Homebrew sbin location, then $PATH, then the literal "dnsmasq".
// Defined once here and reused by serve.go so the path can never drift.
func dnsmasqBinary() string {
	const brew = "/opt/homebrew/sbin/dnsmasq"
	if _, err := os.Stat(brew); err == nil {
		return brew
	}
	if path, err := exec.LookPath("dnsmasq"); err == nil {
		return path
	}
	return "dnsmasq"
}

// pkillBinary resolves the pkill path used by KillStaleDnsmasq. Falls back to
// the conventional macOS/Linux location when $PATH lookup fails.
func pkillBinary() string {
	if path, err := exec.LookPath("pkill"); err == nil {
		return path
	}
	return "/usr/bin/pkill"
}

// visudoBinary resolves the visudo path for syntax validation.
func visudoBinary() string {
	if path, err := exec.LookPath("visudo"); err == nil {
		return path
	}
	return "/usr/sbin/visudo"
}

// sudoersDnsmasqLine returns the NOPASSWD rule granting user the right to run
// the dnsmasq binary (with any args) as root. This exact line is the
// idempotency key for SudoersInstalled().
func sudoersDnsmasqLine(user, dnsmasqPath string) string {
	return fmt.Sprintf("%s ALL=(root) NOPASSWD: %s *", user, dnsmasqPath)
}

// SudoersDropInContent returns the scoped sudoers drop-in granting user
// passwordless root for exactly two things: running the dnsmasq binary (any
// args) and the dnsmasq-scoped pkill used to reap stale servers. Pure: no I/O.
func SudoersDropInContent(user, dnsmasqPath, pkillPath string) string {
	var b strings.Builder
	b.WriteString(sudoersMarker)
	b.WriteString("\n")
	b.WriteString(sudoersDnsmasqLine(user, dnsmasqPath))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s ALL=(root) NOPASSWD: %s -f dnsmasq*", user, pkillPath))
	b.WriteString("\n")
	return b.String()
}

// sudoersInstalledAt reports whether the file at path exists and contains the
// given marker line. Read errors (missing file, permission) are treated as
// "not installed". Pure w.r.t. injectable path so it can be unit-tested
// without touching real /etc.
func sudoersInstalledAt(path, marker string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == marker {
			return true
		}
	}
	return false
}

// SudoersInstalled reports whether the managed drop-in exists AND already
// grants the current user NOPASSWD for the current dnsmasq path. Tolerant of
// read errors (returns false). Exported for reuse by doctor + the serve daemon.
func SudoersInstalled() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	return sudoersInstalledAt(SudoersDropInPath, sudoersDnsmasqLine(u.Username, dnsmasqBinary()))
}

// InstallSudoers writes, validates, and installs the scoped NOPASSWD drop-in
// so `nostos pxe` (serve) can run dnsmasq without a password prompt.
//
// Side-effecting steps are isolated here; the pure content + matching logic
// (SudoersDropInContent, sudoersInstalledAt) is unit-tested without root.
//
// Steps:
//  1. build drop-in content for the current user
//  2. write a 0600 temp file
//  3. `visudo -cf <tmp>` syntax check (no root needed)
//  4. `sudo install -m 0440 -o root -g wheel <tmp> <path>` (the one-time prompt)
//  5. confirm via SudoersInstalled()
func InstallSudoers(stdout io.Writer) error {
	u, err := user.Current()
	if err != nil {
		return errs.Internal("E_PXE_SETUP_USER", "cannot resolve current user: "+err.Error())
	}
	dnsmasqPath := dnsmasqBinary()
	content := SudoersDropInContent(u.Username, dnsmasqPath, pkillBinary())

	tmp, err := os.CreateTemp("", "nostos-pxe-sudoers-*")
	if err != nil {
		return errs.Internal("E_PXE_SETUP_TEMP", "cannot create temp file: "+err.Error())
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return errs.Internal("E_PXE_SETUP_TEMP", "cannot chmod temp file: "+err.Error())
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return errs.Internal("E_PXE_SETUP_TEMP", "cannot write temp file: "+err.Error())
	}
	if err := tmp.Close(); err != nil {
		return errs.Internal("E_PXE_SETUP_TEMP", "cannot close temp file: "+err.Error())
	}

	// Validate syntax. visudo -cf does not require root.
	vcmd := exec.Command(visudoBinary(), "-cf", tmpPath)
	if vout, err := vcmd.CombinedOutput(); err != nil {
		return errs.Validation("E_PXE_SETUP_VISUDO",
			"visudo rejected the generated drop-in: "+strings.TrimSpace(string(vout))).
			WithHint("this is a bug in nostos; the generated sudoers syntax is invalid")
	}

	// Install with root ownership. This sudo call is the one-time interactive
	// password prompt — wire the real os streams so a human can answer it.
	install := exec.Command("sudo", "install", "-m", "0440", "-o", "root", "-g", "wheel", tmpPath, SudoersDropInPath)
	install.Stdin = os.Stdin
	install.Stdout = os.Stderr // keep stdout clean for the JSON/text result
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return errs.Auth("E_PXE_SETUP_SUDO",
			"failed to install sudoers drop-in via sudo: "+err.Error()).
			WithDetails(map[string]any{"path": SudoersDropInPath}).
			WithHint("re-run in an interactive terminal and enter your password when prompted")
	}

	if !SudoersInstalled() {
		return errs.Internal("E_PXE_SETUP_VERIFY",
			"installed drop-in but verification did not find the expected rule").
			WithDetails(map[string]any{"path": SudoersDropInPath})
	}
	return nil
}
