package utils

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

type VersionInfo struct {
	UUID       string
	OldVersion string
	NewVersion string
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
		fmt.Printf("Failed to connect to %s: %s\n", ip, err)
		return
	}
	defer client.Close()

	fmt.Printf("Successfully connected to %s\n", ip)
}
