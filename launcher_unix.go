//go:build !windows

package main

import (
	"os/exec"
)

// spawnHidden on Unix-like systems simply starts the process in the background.
// There is no visible terminal window in typical GUI app launchers.
func spawnHidden(nodePath, mainJS, _ string, env []string) error {
	cmd := exec.Command(nodePath, mainJS)
	cmd.Env = env
	return cmd.Start()
}
