// Package execx provides a small subprocess seam so the rest of nostos
// can invoke external binaries through an interface that tests can fake.
package execx

import (
	"context"
	"io"
	"os/exec"
)

// Commander runs an external command. Implementations MUST honor the
// supplied context for cancellation and MUST NOT alter argv or env.
type Commander interface {
	Run(ctx context.Context, name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error
}

// OSCommander is the default Commander; it wraps exec.CommandContext.
type OSCommander struct{}

// Run executes name with args. If env is non-nil it is set verbatim on
// the child (no inheritance). If env is nil, the child inherits the
// parent's environment.
func (OSCommander) Run(ctx context.Context, name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
