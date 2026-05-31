package upgrade

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DetectVersion shells out to `talosctl version` against nodeIP and returns the
// running SERVER Talos version. It wraps exec like registry.Apply: it looks up
// talosctl on PATH and bounds the call with a context timeout.
func DetectVersion(talosconfigPath, nodeIP string) (Version, error) {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return Version{}, fmt.Errorf("talosctl not found on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{
		"version",
		"--nodes", nodeIP,
		"--endpoints", nodeIP,
		"--talosconfig", talosconfigPath,
		"--short",
	}
	out, err := exec.CommandContext(ctx, "talosctl", args...).CombinedOutput()
	if err != nil {
		return Version{}, fmt.Errorf("talosctl version (%s): %s", nodeIP, strings.TrimSpace(string(out)))
	}
	return parseServerVersion(string(out))
}

// parseServerVersion extracts the SERVER Talos version from `talosctl version`
// output (handles both --short "Server:\n\tTag: v1.10.3" forms and a single
// "Server: v1.10.3" line). Pure helper for testability.
func parseServerVersion(out string) (Version, error) {
	lines := strings.Split(out, "\n")
	inServer := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Server:"):
			inServer = true
			// "Server: v1.10.3" single-line form.
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "Server:"))
			if rest != "" {
				if v, err := ParseVersion(rest); err == nil {
					return v, nil
				}
			}
		case strings.HasPrefix(trimmed, "Client:"):
			inServer = false
		case inServer && strings.HasPrefix(trimmed, "Tag:"):
			tag := strings.TrimSpace(strings.TrimPrefix(trimmed, "Tag:"))
			return ParseVersion(tag)
		}
	}
	return Version{}, fmt.Errorf("could not find server version in talosctl output")
}

// Upgrade runs `talosctl upgrade` against a running node, pulling the given
// installer image. The image path uses /installer/ (running-node upgrades),
// distinct from the /metal-installer/ path used at first install time.
func Upgrade(ctx context.Context, talosconfigPath, nodeIP, image string) error {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return fmt.Errorf("talosctl not found on PATH")
	}
	args := []string{
		"upgrade",
		"--nodes", nodeIP,
		"--endpoints", nodeIP,
		"--talosconfig", talosconfigPath,
		"--image", image,
	}
	out, err := exec.CommandContext(ctx, "talosctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("talosctl upgrade (%s): %s", nodeIP, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallerImage builds the running-node upgrade installer image reference.
func InstallerImage(schematic string, v Version) string {
	return fmt.Sprintf("factory.talos.dev/installer/%s:%s", schematic, v.String())
}

// WaitHealthy polls DetectVersion until the node reports want (and is
// reachable), or the timeout elapses. progress is called with friendly status
// lines; it may be nil.
func WaitHealthy(talosconfigPath, nodeIP string, want Version, timeout, interval time.Duration, progress func(string)) error {
	deadline := time.Now().Add(timeout)
	for {
		got, err := DetectVersion(talosconfigPath, nodeIP)
		if err == nil && got == want {
			if progress != nil {
				progress(fmt.Sprintf("✓ %s is healthy at %s", nodeIP, want))
			}
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("timeout waiting for %s to reach %s: %v", nodeIP, want, err)
			}
			return fmt.Errorf("timeout waiting for %s to reach %s (last seen %s)", nodeIP, want, got)
		}
		if progress != nil {
			if err != nil {
				progress(fmt.Sprintf("  … %s not reachable yet, retrying", nodeIP))
			} else {
				progress(fmt.Sprintf("  … %s at %s, waiting for %s", nodeIP, got, want))
			}
		}
		time.Sleep(interval)
	}
}
