package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct bound to the Wails frontend.
type App struct {
	ctx context.Context

	// gitHost / gitUsername / gitToken are set by the OAuth flow (oauth.go) and
	// consumed during installation step 5 to store credentials in the system
	// credential manager.
	gitHost     string
	gitUsername string
	gitToken    string
}

// NewApp creates a new App instance.
func NewApp() *App { return &App{} }

// startup is called by Wails when the application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Quit closes the application window.
func (a *App) Quit() {
	wailsruntime.Quit(a.ctx)
}

// StepEvent is the shape of the "step" event emitted during installation.
type StepEvent struct {
	Step    int    `json:"step"`
	Total   int    `json:"total"`
	Name    string `json:"name"`
	Status  string `json:"status"`  // "running" | "done" | "error"
	Message string `json:"message"` // sub-message / log line
}

// GetDefaultInstallPath returns the OS-appropriate default install path.
func (a *App) GetDefaultInstallPath() string {
	return defaultInstallPath()
}

// GetExistingInstallPath returns the previously configured install root,
// or "" if the app has never been installed on this machine.
func (a *App) GetExistingInstallPath() string {
	if root, err := readDominionConfig(); err == nil {
		return root
	}
	return ""
}

// ChooseDirectory shows the OS native folder-picker dialog.
// Returns the chosen path, or "" if the user cancelled.
func (a *App) ChooseDirectory(defaultPath string) string {
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		DefaultDirectory: defaultPath,
		Title:            "Choose install location for The Dominion",
	})
	if err != nil || path == "" {
		return ""
	}
	return path
}

// SaveGitCredential stores PAT credentials for a custom ("Other") git provider.
// Returns an error message, or "" on success. Used only when OAuth is not available.
func (a *App) SaveGitCredential(host, username, token string) string {
	if host == "" || username == "" || token == "" {
		return "Host, username, and token are all required."
	}
	a.gitHost = host
	a.gitUsername = username
	a.gitToken = token
	return ""
}

// StartInstallation kicks off the installation in a background goroutine.
// Progress is communicated to the frontend via "step", "done", and "error" events.
func (a *App) StartInstallation(installPath string) {
	go a.runInstallation(installPath)
}

// OpenURL opens a URL in the default system browser.
func (a *App) OpenURL(url string) {
	_ = openBrowser(url)
}

// ── installation ──────────────────────────────────────────────────────────────

const totalSteps = 8

var stepNames = [totalSteps]string{
	"Configure install location",
	"Install Node.js",
	"Install Git",
	"Install Claude Code",
	"Configure git credentials",
	"Download Dominion runtime",
	"Launch Dominion",
	"Register auto-start",
}

func (a *App) emitStep(step int, status, message string) {
	wailsruntime.EventsEmit(a.ctx, "step", StepEvent{
		Step:    step,
		Total:   totalSteps,
		Name:    stepNames[step-1],
		Status:  status,
		Message: message,
	})
}

// logger returns a func(string) that emits a "running" step event with the message.
func (a *App) logger(step int) func(string) {
	return func(msg string) {
		a.emitStep(step, "running", msg)
	}
}

func (a *App) runInstallation(installPath string) {
	// Step 1 — Persist config
	a.emitStep(1, "running", "")
	if err := persistDominionConfig(installPath); err != nil {
		a.emitStep(1, "error", err.Error())
		wailsruntime.EventsEmit(a.ctx, "error", err.Error())
		return
	}
	a.emitStep(1, "done", "")

	// Step 2 — Ensure Node.js (+ pnpm via Volta)
	a.emitStep(2, "running", "")
	if err := ensureNode(a.logger(2)); err != nil {
		a.emitStep(2, "error", err.Error())
		wailsruntime.EventsEmit(a.ctx, "error", err.Error())
		return
	}
	a.emitStep(2, "done", "")

	// Step 3 — Ensure Git (Git Bash on Windows; required by Claude Code)
	a.emitStep(3, "running", "")
	if err := ensureGit(a.logger(3)); err != nil {
		a.emitStep(3, "error", err.Error())
		wailsruntime.EventsEmit(a.ctx, "error", err.Error())
		return
	}
	a.emitStep(3, "done", "")

	// Step 4 — Install Claude Code via native installer
	a.emitStep(4, "running", "")
	if err := ensureClaudeCode(a.logger(4)); err != nil {
		a.emitStep(4, "error", err.Error())
		wailsruntime.EventsEmit(a.ctx, "error", err.Error())
		return
	}
	a.emitStep(4, "done", "")

	// Step 5 — Store git credentials (set by OAuth / PAT screen; non-fatal if skipped)
	a.emitStep(5, "running", "")
	if a.gitHost != "" {
		if err := configureGitCredentials(a.gitHost, a.gitUsername, a.gitToken, a.logger(5)); err != nil {
			a.emitStep(5, "done", fmt.Sprintf("Warning: %v", err))
		} else {
			a.emitStep(5, "done", "")
		}
	} else {
		a.emitStep(5, "done", "Skipped")
	}

	// Step 6 — Download Dominion runtime
	dominionDir := filepath.Join(installPath, "__tools")
	a.emitStep(6, "running", "")
	if err := downloadDominion(dominionDir, a.logger(6)); err != nil {
		a.emitStep(6, "error", err.Error())
		wailsruntime.EventsEmit(a.ctx, "error", err.Error())
		return
	}
	a.emitStep(6, "done", "")

	// Step 7 — Launch Dominion
	mainJS := filepath.Join(dominionDir, "main.js")
	a.emitStep(7, "running", "")
	port, err := launchDominion(mainJS, installPath, a.logger(7))
	if err != nil {
		a.emitStep(7, "error", err.Error())
		wailsruntime.EventsEmit(a.ctx, "error", err.Error())
		return
	}
	a.emitStep(7, "done", "")

	// Step 8 — Register auto-start (non-fatal)
	a.emitStep(8, "running", "")
	if err := registerLoginItem(mainJS, installPath); err != nil {
		a.emitStep(8, "done", fmt.Sprintf("Warning: %v", err))
	} else {
		a.emitStep(8, "done", "")
	}

	dashboardURL := fmt.Sprintf("http://localhost:%d", port)
	wailsruntime.EventsEmit(a.ctx, "done", dashboardURL)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func defaultInstallPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if _, err := os.Stat(`D:\`); err == nil {
			return `D:\Minions`
		}
		return filepath.Join(home, "Minions")
	default:
		return filepath.Join(home, "Minions")
	}
}
