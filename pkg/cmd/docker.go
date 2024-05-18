package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/pkg/utils"
)

// Docker run command
var dockerExecCmd = &cobra.Command{
	Use:   "exec",
	Short: "TODO",
	Long:  `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		nctx.ExecuteCommand(
			"docker", "run", "-it", "--rm", "--entrypoint=fish",
			"-v", "ssh:/root/.ssh",
			"-v", "./secrets:/etc/secrets",
			"-v", fmt.Sprintf(".:/%s", dockerWorkdir), image)
	},
}

// Docker run command
var dockerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Docker container",
	Long:  `Runs a Docker container with specified arguments.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Basic Docker run command arguments
		dockerArgs := []string{
			"run", "--rm",
			"-v", "ssh:/root/.ssh",
			"-v", "./secrets:/etc/secrets",
			"-v", fmt.Sprintf(".:/%s", dockerWorkdir),
			image,
		}

		// Append additional arguments to be executed in the Docker container
		dockerArgs = append(dockerArgs, args...)
		fmt.Println(dockerArgs)
		// Execute the Docker command with the appended arguments
		nctx.ExecuteCommand("docker", dockerArgs...)
	},
}

// Docker build command
var dockerBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Docker image",
	Long:  `Builds a Docker image from the specified Dockerfile.`,
	Run: func(cmd *cobra.Command, args []string) {
		nctx.ExecuteCommand("docker", "build", "-t", image, ".")
	},
}

// Docker command group
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Docker operations from the builder machine",
	Long:  `Commands to manage Docker containers.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		nctx = utils.NewContext(currentWorkdir, verbose)
	},
}
