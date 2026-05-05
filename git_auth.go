package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// configureGitCredentials ensures the system credential helper is set, then
// stores the provided credentials so that git clone/push work without prompting.
func configureGitCredentials(host, username, token string, log func(string)) error {
	log("Configuring git credential helper…")
	if err := ensureCredentialHelper(log); err != nil {
		// Non-fatal: log the warning and try to store credentials anyway.
		log(fmt.Sprintf("Warning: could not configure credential helper: %v", err))
	}

	log(fmt.Sprintf("Storing credentials for %s…", host))
	return storeGitCredential(host, username, token)
}

// ensureCredentialHelper sets a platform-appropriate credential helper if none
// is already configured globally.
func ensureCredentialHelper(log func(string)) error {
	out, err := exec.Command("git", "config", "--global", "credential.helper").Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		log("Credential helper already configured.")
		return nil
	}

	var helper string
	switch runtime.GOOS {
	case "windows":
		// Git Credential Manager is bundled with Git for Windows.
		helper = "manager"
	case "darwin":
		helper = "osxkeychain"
	default:
		// Plaintext file fallback on Linux — better than nothing.
		helper = "store"
	}

	log(fmt.Sprintf("Setting credential helper to %q…", helper))
	cmd := exec.Command("git", "config", "--global", "credential.helper", helper)
	if combined, runErr := cmd.CombinedOutput(); runErr != nil {
		return fmt.Errorf("git config --global credential.helper %s: %w\n%s", helper, runErr, combined)
	}
	return nil
}

// storeGitCredential passes credentials to the configured git credential helper
// via "git credential approve".
func storeGitCredential(host, username, password string) error {
	input := fmt.Sprintf(
		"protocol=https\nhost=%s\nusername=%s\npassword=%s\n\n",
		host, username, password,
	)
	cmd := exec.Command("git", "credential", "approve")
	cmd.Stdin = strings.NewReader(input)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git credential approve: %w\n%s", err, out)
	}
	return nil
}
