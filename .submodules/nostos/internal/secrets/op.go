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
// OnePasswordBackend resolves op://vault/item/field via the 1Password CLI (`op`).
type OnePasswordBackend struct {
	Account string
	runner  CommandRunner // injectable for tests
}

// NewOnePassword constructs a backend tied to an op account.
func NewOnePassword(account string) *OnePasswordBackend {
	return &OnePasswordBackend{Account: account, runner: defaultRunner{}}
}

func (b *OnePasswordBackend) Scheme() string { return "op" }

func (b *OnePasswordBackend) env() []string {
	env := os.Environ()
	if b.Account != "" {
		env = append(env, "OP_ACCOUNT="+b.Account)
	}
	return env
}

func (b *OnePasswordBackend) Validate() error {
	if _, err := exec.LookPath("op"); err != nil {
		return &ResolveError{URI: "op://...", Reason: "1Password CLI not found; install via: brew install 1password-cli"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := b.runner.Run(ctx, b.env(), "op", "whoami")
	if err != nil {
		stderr := strings.ToLower(string(out))
		if strings.Contains(stderr, "not signed in") {
			return &ResolveError{URI: "op://...", Reason: "1Password session not active; run: op signin"}
		}
		return &ResolveError{URI: "op://...", Reason: fmt.Sprintf("op whoami failed: %s", strings.TrimSpace(string(out)))}
	}
	return nil
}

func (b *OnePasswordBackend) Resolve(uri string) (string, error) {
	if !strings.HasPrefix(uri, "op://") {
		return "", &ResolveError{URI: uri, Reason: "not an op:// URI"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := b.runner.RunInteractive(ctx, b.env(), "op", "read", uri)
	if err != nil {
		// Never leak stdout; it may contain partial secret. Keep only stderr fragments.
		// exec.ExitError exposes Stderr when cmd.Stderr is nil; when we pipe to os.Stderr
		// above, it's shown to the user directly, so our msg is generic.
		msg := strings.TrimSpace(string(bytes.Split(out, []byte("\n"))[0]))
		if msg == "" {
			msg = fmt.Sprintf("op read failed: %v", err)
		}
		return "", &ResolveError{URI: uri, Reason: msg}
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}
