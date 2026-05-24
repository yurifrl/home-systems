// Package execxtest provides a deterministic Commander double for tests.
package execxtest

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// Call records a single Run invocation.
type Call struct {
	Name  string
	Args  []string
	Env   []string
	Stdin []byte
}

// Script controls the fake's response for a particular Run.
type Script struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// FakeCommander implements execx.Commander deterministically.
type FakeCommander struct {
	mu      sync.Mutex
	scripts []Script
	idx     int
	Calls   []Call
}

// New returns a FakeCommander that will replay the given scripts in order.
// If more Run calls happen than scripts provided, the last script is reused.
// If no scripts are provided, Run returns nil with empty output.
func New(scripts ...Script) *FakeCommander {
	return &FakeCommander{scripts: scripts}
}

// Run records the invocation, copies stdin, writes scripted output and
// returns the scripted error.
func (f *FakeCommander) Run(ctx context.Context, name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	c := Call{Name: name}
	if args != nil {
		c.Args = append([]string{}, args...)
	}
	if env != nil {
		c.Env = append([]string{}, env...)
	}
	if stdin != nil {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("execxtest: read stdin: %w", err)
		}
		c.Stdin = b
	}
	f.Calls = append(f.Calls, c)

	var s Script
	switch {
	case len(f.scripts) == 0:
		s = Script{}
	case f.idx < len(f.scripts):
		s = f.scripts[f.idx]
		f.idx++
	default:
		s = f.scripts[len(f.scripts)-1]
	}

	if stdout != nil && len(s.Stdout) > 0 {
		if _, err := stdout.Write(s.Stdout); err != nil {
			return err
		}
	}
	if stderr != nil && len(s.Stderr) > 0 {
		if _, err := stderr.Write(s.Stderr); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Err
}
