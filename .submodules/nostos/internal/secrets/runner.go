package secrets

import (
	"context"
	"os"
	"os/exec"
)

// CommandRunner runs a subprocess and returns combined output. Abstracted so
// tests can inject fakes that never shell out to real tools.
type CommandRunner interface {
	Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	RunInteractive(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
}

type defaultRunner struct{}

func (defaultRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	return cmd.CombinedOutput()
}

// RunInteractive pipes stdin + stderr to the operator's TTY so subprocesses
// that need biometric or password prompts (op, sops) work correctly. stdout
// is captured so callers can read resolved secret values.
func (defaultRunner) RunInteractive(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Output()
}
