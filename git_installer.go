package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// ensureGit checks for an existing Git installation and installs it if absent.
// On Windows, this installs Git for Windows (which includes Git Bash — required
// by Claude Code on Windows). On macOS, it uses Homebrew or Xcode CLT.
func ensureGit(log func(string)) error {
	if _, err := exec.LookPath("git"); err == nil {
		log("Git is already installed.")
		return nil
	}

	log("Git not found. Installing…")

	switch runtime.GOOS {
	case "windows":
		return installGitWindows(log)
	case "darwin":
		return installGitMac(log)
	default:
		return fmt.Errorf(
			"automatic Git installation not supported on %s; please install Git manually",
			runtime.GOOS,
		)
	}
}

func installGitWindows(log func(string)) error {
	log("Installing Git for Windows via winget (includes Git Bash)…")
	cmd := exec.Command("winget", "install", "--id", "Git.Git",
		"--exact", "--silent",
		"--scope", "user",
		"--accept-source-agreements",
		"--accept-package-agreements",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("winget install Git.Git: %w\n%s", err, out)
	}
	log("Git for Windows installed.")
	return nil
}

func installGitMac(log func(string)) error {
	// Prefer Homebrew if available.
	if _, err := exec.LookPath("brew"); err == nil {
		log("Installing Git via Homebrew…")
		cmd := exec.Command("brew", "install", "git")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("brew install git: %w\n%s", err, out)
		}
		log("Git installed via Homebrew.")
		return nil
	}

	// Fall back to Xcode Command Line Tools (which bundle git).
	log("Installing Xcode Command Line Tools (includes Git)…")
	_ = exec.Command("xcode-select", "--install").Run()

	// Poll for git to become available (CLT install is asynchronous/GUI-driven).
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := exec.LookPath("git"); err == nil {
			log("Git installed via Xcode CLT.")
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("git not found after Xcode CLT prompt; please install Git manually and re-run")
}
