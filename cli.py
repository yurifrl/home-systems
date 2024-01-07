#!/usr/bin/env python3
import click
import subprocess
import shlex


class Options:
    def __init__(self):
        self.verbose = False


image = "nixos-sd-image"
docker_dir = "./image"


def subprocess_run(ctx, *args, **kwargs):
    options = ctx.obj
    command_string = " ".join(shlex.quote(str(arg)) for arg in args[0])
    click.secho(f"Executing command: {command_string}", fg="blue")
    try:
        subprocess.run(*args, **kwargs)
    except subprocess.CalledProcessError as e:
        if options.verbose:
            raise
        else:
            click.secho(f"Error: command: {command_string}", fg="red")


@click.group()
@click.option("-v", "--verbose", is_flag=True, help="Enable verbose output.")
@click.pass_context
def cli(ctx, verbose):
    if not hasattr(ctx, "options"):
        ctx.options = Options()
    ctx.options.verbose = verbose


@cli.group()
@click.pass_context
def docker(ctx):
    pass


@docker.command(name="build")
@click.pass_context
def docker_build(ctx):
    subprocess_run(
        ctx,
        ["docker", "build", "-t", image, "."],
        cwd=docker_dir,
        check=True,
    )


@docker.command(name="run")
@click.pass_context
def docker_run(ctx):
    subprocess_run(
        ctx,
        [
            "docker",
            "run",
            "-it",
            "--rm",
            "-v",
            ".:/workdir",
            "--entrypoint",
            "bash",
            image,
        ],
        cwd=docker_dir,
        check=True,
    )


if __name__ == "__main__":
    cli(obj=Options())
