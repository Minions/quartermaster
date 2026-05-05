package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ensureClaudeCode installs the Claude Code CLI using the official native
// installer so that the built-in auto-updater can locate and replace the binary
// at its expected default path. Installing via pnpm/Volta puts the binary in
// the wrong location and causes auto-updates to break.
func ensureClaudeCode(log func(string)) error {
	if path, err := findClaude(); err == nil {
		log(fmt.Sprintf("Claude Code is already installed at %s.", path))
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		return installClaudeWindows(log)
	default:
		return installClaudeUnix(log)
	}
}

// installClaudeWindows runs the official PowerShell native installer.
// This installs claude.exe to Claude's default location so auto-updates work.
func installClaudeWindows(log func(string)) error {
	log("Running Claude Code native installer for Windows…")
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", "irm https://claude.ai/install.ps1 | iex")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Claude Code installer: %w\n%s", err, out)
	}
	log("Claude Code installed.")
	return nil
}

// installClaudeUnix runs the official shell native installer.
// This installs claude to ~/.local/bin/claude so auto-updates work.
func installClaudeUnix(log func(string)) error {
	log("Running Claude Code native installer…")
	cmd := exec.Command("bash", "-c", "curl -fsSL https://claude.ai/install.sh | bash")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Claude Code installer: %w\n%s", err, out)
	}
	log("Claude Code installed.")
	return nil
}

// findClaude locates the Claude Code binary. It checks the native installer's
// default paths before falling back to PATH.
func findClaude() (string, error) {
	home, _ := os.UserHomeDir()

	// Check the native installer's default location first.
	var nativePath string
	switch runtime.GOOS {
	case "windows":
		// The Windows native installer places the binary in %LOCALAPPDATA%\AnthropicClaude.
		nativePath = filepath.Join(os.Getenv("LOCALAPPDATA"), "AnthropicClaude", "claude.exe")
	default:
		// The Unix native installer places the binary in ~/.local/bin.
		nativePath = filepath.Join(home, ".local", "bin", "claude")
	}
	if _, err := os.Stat(nativePath); err == nil {
		return nativePath, nil
	}

	return exec.LookPath("claude")
}
