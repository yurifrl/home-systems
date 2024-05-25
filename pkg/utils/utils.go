package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
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

// Extract and parse JSON from the output
func ExtractAndParseJSON(output string) ([]map[string]interface{}, error) {
	jsonStart := strings.Index(output, "[")
	jsonEnd := strings.LastIndex(output, "]") + 1

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("failed to find JSON in output: %s", output)
	}

	jsonOutput := output[jsonStart:jsonEnd]

	var result []map[string]interface{}
	err := json.Unmarshal([]byte(jsonOutput), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %s", err)
	}

	return result, nil
}

// Get output paths from parsed JSON
func GetOutputPaths(parsedJSON []map[string]interface{}) []string {
	var outputPaths []string
	for _, item := range parsedJSON {
		if outputs, ok := item["outputs"].(map[string]interface{}); ok {
			if out, ok := outputs["out"].(string); ok {
				out = fmt.Sprintf("%s/sd-image/", out)
				outputPaths = append(outputPaths, out)
			}
		}
	}
	return outputPaths
}

// HandleBuildArtifacts copies all contents from directories listed in outputPaths to the distDir.
// It ensures the destination directory exists and handles any errors encountered during the process.
func HandleBuildArtifacts(outputPaths []string, distDir string) error {
	for _, srcDir := range outputPaths {
		err := CopyDirContents(srcDir, distDir)
		if err != nil {
			return fmt.Errorf("failed to copy contents of %s: %v", srcDir, err)
		}
	}
	return nil
}

// CopyDirContents copies all files and directories from srcDir to distDir.
// It creates the destination directory if it doesn't exist and overrides existing files.
func CopyDirContents(srcDir, distDir string) error {
	err := os.MkdirAll(distDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create destination directory %s: %v", distDir, err)
	}

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create the destination path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(distDir, relPath)

		// Copy directory
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy file
		return CopyFile(path, destPath, info)
	})

	if err != nil {
		return fmt.Errorf("failed to copy directory contents from %s to %s: %v", srcDir, distDir, err)
	}

	return nil
}

// CopyFile copies a single file from src to dst and preserves the original file permissions.
func CopyFile(src, dst string, info os.FileInfo) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %v", src, err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %v", dst, err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy contents from %s to %s: %v", src, dst, err)
	}

	// Copy file permissions
	if err := os.Chmod(dst, info.Mode()); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %v", dst, err)
	}

	// Copy file ownership
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := os.Chown(dst, int(stat.Uid), int(stat.Gid)); err != nil {
			return fmt.Errorf("failed to set ownership on %s: %v", dst, err)
		}
	}

	return nil
}
