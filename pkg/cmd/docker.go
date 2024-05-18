package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/pkg/utils"
)

// Docker command group
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Docker operations from the builder machine",
	Long:  `Commands to manage Docker containers.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		nctx = utils.NewContext(workdir, verbose)
	},
}

// Docker run command
var dockerExecCmd = &cobra.Command{
	Use:   "exec",
	Short: "TODO",
	Long:  `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		nctx.ExecuteCommand(
			"docker", "run", "-it", "--rm", "--entrypoint=fish",
			"-v", "ssh:/root/.ssh",
			"-v", "nixops:/nixops",
			"-v", "./secrets:/etc/secrets",
			"-v", ".:/workdir", image)
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
			"-v", "nixops:/nixops",
			"-v", "./secrets:/etc/secrets",
			"-v", ".:/workdir",
			image,
		}

		// Append additional arguments to be executed in the Docker container
		for _, arg := range args {
			dockerArgs = append(dockerArgs, arg)
		}
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
