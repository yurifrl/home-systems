package executors

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LocalExecutor struct {
}

func (LocalExecutor) ExecuteCommand(name string, args ...string) (bytes.Buffer, error) {
	executor := NewExecutor()
	var out bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Dir = filepath.Join(".", executor.currentWorkdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	fmt.Printf("Executing command: `%s %s`\n", name, strings.Join(args, " "))
	return out, cmd.Run()
}
