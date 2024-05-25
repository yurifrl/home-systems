package executors

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type LocalExecutor struct {
}

func (LocalExecutor) ExecuteCommand(name string, args ...string) (err error) {
	// executor := NewExecutor()

	cmd := exec.Command(name, args...)
	cmd.Dir = "./nix"
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Executing command: `%s %s`\n", name, strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error executing command: %s\n", err)
	}
	return
}
