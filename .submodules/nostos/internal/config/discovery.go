package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindConfig resolves the active config.yaml path.
//
// Search order:
//  1. explicit (from --config flag)
//  2. $NOSTOS_CONFIG
//  3. ./config.yaml in the current working directory
//  4. walk up from cwd looking for nostos/config.yaml
func FindConfig(explicit, cwd string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(os.ExpandEnv(explicit))
		if err != nil {
			return "", fmt.Errorf("--config path: %w", err)
		}
		if !fileExists(abs) {
			return "", fmt.Errorf("--config path does not exist: %s", abs)
		}
		return abs, nil
	}

	if env := os.Getenv("NOSTOS_CONFIG"); env != "" {
		abs, err := filepath.Abs(os.ExpandEnv(env))
		if err != nil {
			return "", fmt.Errorf("$NOSTOS_CONFIG: %w", err)
		}
		if !fileExists(abs) {
			return "", fmt.Errorf("$NOSTOS_CONFIG points to missing file: %s", abs)
		}
		return abs, nil
	}

	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	start, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	cwdConfig := filepath.Join(start, "config.yaml")
	if fileExists(cwdConfig) {
		return cwdConfig, nil
	}

	current := start
	for {
		candidate := filepath.Join(current, "nostos", "config.yaml")
		if fileExists(candidate) {
			return candidate, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", fmt.Errorf(
		"no config.yaml found (checked --config, $NOSTOS_CONFIG, %s, and nostos/config.yaml in every parent of %s). Run `nostos init` to create one",
		cwdConfig, start,
	)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
