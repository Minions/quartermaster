//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// spawnHidden starts a Node.js process on Windows with no visible console window.
// Stdout and stderr are redirected to dominion.log in workDir for diagnostics.
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
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000200, // CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP
	}
	return cmd.Start()
}
