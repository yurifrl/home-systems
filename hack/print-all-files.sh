#!/usr/bin/env bash

# Check if a path is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <path>"
  exit 1
fi

# Get the provided path
directory=$1

# Hardcoded list of directories to ignore
ignored=("vendor" ".git" "dist" "go.sum" "flake.lock" "secrets" ".gitignore" ".vscode" "docker-compose.yml" "hack" "renovate.json" "Dockerfile")

# Build the find command with prune conditions
prune_conditions=""
for dir in "${ignored[@]}"; do
  prune_conditions+=" -path $directory/$dir -o"
done
# Remove the trailing -o
prune_conditions="${prune_conditions::-2}"

files_cmd="find \"$directory\" \($prune_conditions\) -prune -o -type f -print"
files=$(eval $files_cmd)

# Loop through each file
for file in $files
do
  echo "#============================"
  echo "# $file"
  echo "#============================"
  echo
  cat "$file"
  echo
done
