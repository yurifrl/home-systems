package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
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
	Short: "Execute command in Docker container",
	Long:  `Executes a command in a Docker container with specific volumes mounted.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDockerExec(); err != nil {
			log.Fatalf("Error running Docker exec: %v", err)
		}
	},
}

// Docker run command
var dockerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Docker container",
	Long:  `Runs a Docker container with specified arguments.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDockerContainer(args); err != nil {
			log.Fatalf("Error running Docker container: %v", err)
		}
	},
}

// Docker build command
var dockerBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Docker image",
	Long:  `Builds a Docker image from the specified Dockerfile.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := buildDockerImage(); err != nil {
			log.Fatalf("Error building Docker image: %v", err)
		}
	},
}

func runDockerExec() error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	ctx := context.Background()
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   []string{"fish"},
		Tty:   true,
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: mount.TypeVolume, Source: "ssh", Target: "/root/.ssh"},
			{Type: mount.TypeVolume, Source: "nixops", Target: "/nixops"},
			{Type: mount.TypeBind, Source: "./secrets", Target: "/etc/secrets"},
			{Type: mount.TypeBind, Source: ".", Target: "/workdir"},
		},
	}, nil, nil, "")
	if err != nil {
		return err
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	fmt.Printf("Container %s started\n", resp.ID)
	return nil
}

func runDockerContainer(args []string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	ctx := context.Background()
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   args,
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: mount.TypeVolume, Source: "ssh", Target: "/root/.ssh"},
			{Type: mount.TypeVolume, Source: "nixops", Target: "/nixops"},
			{Type: mount.TypeBind, Source: "./secrets", Target: "/etc/secrets"},
			{Type: mount.TypeBind, Source: ".", Target: "/workdir"},
		},
	}, nil, nil, "")
	if err != nil {
		return err
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	fmt.Printf("Container %s started with args %v\n", resp.ID, args)
	return nil
}

func buildDockerImage() error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	ctx := context.Background()
	dockerBuildContext, err := os.Open(".")
	if err != nil {
		return err
	}
	defer dockerBuildContext.Close()

	buildOptions := types.ImageBuildOptions{
		Context:    dockerBuildContext,
		Dockerfile: "Dockerfile",
		Tags:       []string{image},
	}

	buildResponse, err := cli.ImageBuild(ctx, dockerBuildContext, buildOptions)
	if err != nil {
		return err
	}
	defer buildResponse.Body.Close()

	fmt.Printf("Successfully built image %s\n", image)
	return nil
}
