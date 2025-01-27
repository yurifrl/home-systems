package executors

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/k0kubun/pp/v3"
)

type DockerExecutor struct {
}

// ExecuteCommand runs a command with given arguments.
func (c *DockerExecutor) ExecuteCommand(name string, args ...string) (_ bytes.Buffer, err error) {
	pp.Println("Running on docker")
	var stdout bytes.Buffer

	executor := NewExecutor()

	dockerArgs := []string{
		"run", "--rm",
		"-v", "ssh:/root/.ssh",
		"-v", fmt.Sprintf("./secrets:%s/secrets", executor.dockerWorkdir),
		"-v", fmt.Sprintf(".:%s", executor.dockerWorkdir),
		executor.image,
	}

	// Append additional arguments to be executed in the Docker container
	args = append([]string{name}, args...)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.Command("docker", dockerArgs...)
	cmd.Dir = filepath.Join(".", executor.currentWorkdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Executing command: `%s %s`\n", "docker", strings.Join(dockerArgs, " "))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error executing command: %s\n", err)
	}
	return stdout, nil
}
