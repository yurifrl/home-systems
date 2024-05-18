package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type Context struct {
	Workdir string
	Verbose bool
}

func NewContext(workdir string, verbose bool) *Context {
	return &Context{
		Workdir: workdir,
		Verbose: verbose,
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

func (c *Context) ExecuteCommand2(name string, args ...string) {
	fmt.Printf("Executing command: `%s %s`\n", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Dir = filepath.Join(c.Workdir)

	// Start the command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf("Failed to start command with pty: %s\n", err)
	}
	defer func() { _ = ptmx.Close() }()

	// Set stdin in a non-blocking mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Failed to set stdin to raw mode: %s\n", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Copy stdin to the pty and the pty to stdout.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	go func() { _, _ = io.Copy(os.Stdout, ptmx) }()

	// Wait for the command to finish
	err = cmd.Wait()
	if err != nil {
		log.Printf("Command finished with error: %s\n", err)
	}
}

// ExecuteCommand runs a command with given arguments.
func (c *Context) ExecuteCommand(name string, args ...string) (err error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = filepath.Join(".", c.Workdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if c.Verbose {
		cmdStr := fmt.Sprintf("Executing command: `%s %s`", name, strings.Join(args, " "))
		fmt.Println(cmdStr)
	}
	if err := cmd.Run(); err != nil {
		if c.Verbose {
			fmt.Printf("Error executing command: %s\n", err)
		} else {
			fmt.Printf("Error: command failed\n")
		}
	}
	return
}
