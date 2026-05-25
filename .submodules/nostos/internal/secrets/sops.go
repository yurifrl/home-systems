package secrets

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SopsBackend resolves sops://<file>#<key> via `sops --decrypt --extract`.
type SopsBackend struct {
	AgeKeyFile string
	runner     CommandRunner
}

// NewSops constructs a sops backend.
func NewSops(ageKeyFile string) *SopsBackend {
	return &SopsBackend{AgeKeyFile: ageKeyFile, runner: defaultRunner{}}
}

func (b *SopsBackend) Scheme() string { return "sops" }

func (b *SopsBackend) env() []string {
	env := os.Environ()
	if b.AgeKeyFile != "" {
		env = append(env, "SOPS_AGE_KEY_FILE="+b.AgeKeyFile)
	}
	return env
}

func (b *SopsBackend) Validate() error {
	if _, err := exec.LookPath("sops"); err != nil {
		return &ResolveError{URI: "sops://...", Reason: "sops CLI not found; install via: brew install sops"}
	}
	return nil
}

func (b *SopsBackend) Resolve(uri string) (string, error) {
	if !strings.HasPrefix(uri, "sops://") {
		return "", &ResolveError{URI: uri, Reason: "not a sops:// URI"}
	}
	body := strings.TrimPrefix(uri, "sops://")
	hash := strings.Index(body, "#")
	if hash < 0 {
		return "", &ResolveError{URI: uri, Reason: "sops URIs must include a fragment: sops://<file>#<key>"}
	}
	file, key := body[:hash], body[hash+1:]
	if _, err := os.Stat(file); err != nil {
		return "", &ResolveError{URI: uri, Reason: fmt.Sprintf("file not found: %s", file)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := b.runner.Run(ctx, b.env(), "sops", "--decrypt", "--extract", "['"+key+"']", file)
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		msg := lines[len(lines)-1]
		if msg == "" {
			msg = "sops failed"
		}
		return "", &ResolveError{URI: uri, Reason: msg}
	}
	return strings.TrimRight(string(bytes.TrimSpace(out)), "\r\n"), nil
}
