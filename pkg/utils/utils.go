package utils

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yurifrl/home-systems/pkg/types"
	"golang.org/x/crypto/ssh"
)

var (
	isosDir = "isos"
	device  = ""
)

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

func Flash(devise string, isoImage string, exec types.Executor) {
	// Code that can go to a function
	if isoImage == "" {
		isoFiles, err := filepath.Glob(filepath.Join(isosDir, "*.img"))
		if err != nil {
			log.Println("Error listing ISO images files:", err)
			return
		}
		if len(isoFiles) == 0 {
			log.Println("No ISO images files found in", isosDir)
			return
		}
		// Sort and display ISO files for user to select
		sort.Strings(isoFiles)
		for i, file := range isoFiles {
			log.Printf("%d: %s\n", i+1, file)
		}
		log.Print("Enter the number of the ISO images file to flash: ")
		var choice int
		fmt.Scanln(&choice)
		if choice < 1 || choice > len(isoFiles) {
			log.Println("Invalid choice")
			return
		}
		isoImage = isoFiles[choice-1]
	}
	comand := []string{"sudo", "dd", "bs=4M", "status=progress", "conv=fsync", "of=" + device, "if=" + isoImage}

	// Prompt user for confirmation before proceeding
	log.Println(strings.Join(comand, " "))
	log.Println()
	log.Printf("Are you sure you want to flash '%s' to '%s'? This will erase all data on the device. Type 'y' to confirm: ", isoImage, device)
	var confirmation string
	fmt.Scanln(&confirmation)
	if confirmation != "y" {
		log.Println("Flash operation cancelled.")
		return
	}
	// exec.ExecuteCommand("diskutil", "unmountDisk", "/dev/disk2")
	// Execute the dd command to flash the ISO to the device
	// exec.executeCommand("sudo", "dd", "bs=4M", "status=progress", "conv=fsync", "of="+device, "if="+isoImage)
}
