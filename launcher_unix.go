//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// spawnHidden on Unix-like systems starts the process in the background,
// redirecting output to dominion.log in workDir for diagnostics.
func spawnHidden(nodePath, mainJS, workDir string, env []string) error {
	logPath := filepath.Join(workDir, "dominion.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create dominion log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(nodePath, mainJS)
	cmd.Dir = workDir
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	return cmd.Start()
}
