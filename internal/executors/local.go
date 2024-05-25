package executors

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/k0kubun/pp/v3"
)

type LocalExecutor struct {
}

func (LocalExecutor) ExecuteCommand(name string, args ...string) (err error) {
	pp.Println("Running locally")

	executor := NewExecutor()

	cmd := exec.Command(name, args...)
	cmd.Dir = filepath.Join(".", executor.currentWorkdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Executing command: `%s %s`\n", name, strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error executing command: %s\n", err)
	}
	return
}
