package utils

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Context struct {
	CurrentWorkdir string
	Verbose        bool
}

func NewContext(currentWorkdir string, verbose bool) *Context {
	return &Context{
		CurrentWorkdir: currentWorkdir,
		Verbose:        verbose,
	}
}
func ScanAddress(ip string) {
	// Define the SSH configuration
	config := &ssh.ClientConfig{
		User: "nixos",
		Auth: []ssh.AuthMethod{
			ssh.Password(""), // Empty password or provide a method of authentication
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Not recommended for production
		Timeout:         5 * time.Second,             // Short timeout as requested
	}

	// Attempt to establish an SSH connection
	client, err := ssh.Dial("tcp", ip+":22", config)
	if err != nil {
		log.Printf("Failed to connect to %s: %s\n", ip, err)
		return
	}
	defer client.Close()

	log.Printf("Successfully connected to %s\n", ip)
}

// ExecuteCommand runs a command with given arguments.
func (c *Context) ExecuteCommand(name string, args ...string) (err error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = filepath.Join(".", c.CurrentWorkdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Executing command: `%s %s`\n", name, strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error executing command: %s\n", err)
	}
	return
}
