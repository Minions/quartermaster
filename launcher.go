package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// voltaVersion is the pinned Volta release used for Node.js installation.
// Update together with the SHA256 variables when upgrading Volta.
const voltaVersion = "1.1.1"

// SHA-256 digests for Volta release assets, embedded at build time via -ldflags.
// If empty (dev builds), integrity checking is skipped.
var (
	voltaWindowsSHA256  string
	voltaMacX86SHA256   string
	voltaMacARM64SHA256 string
)

// launchDominion starts the Dominion node process with a hidden console window
// (Windows) or as a background process (Mac/Linux).
// Returns the port the Dominion is configured to use.
func launchDominion(mainJS string, dominionRoot string, log func(string)) (int, error) {
	nodePath, err := findNode()
	if err != nil {
		return 0, fmt.Errorf("node not found: %w", err)
	}

	env := append(os.Environ(),
		fmt.Sprintf("DOMINION_ROOT=%s", dominionRoot),
	)

	log("Starting Dominion process…")
	if err := spawnHidden(nodePath, mainJS, dominionRoot, env); err != nil {
		return 0, fmt.Errorf("spawn dominion: %w", err)
	}

	// Wait for Dominion to come up (poll /health)
	port := 3535
	deadline := time.Now().Add(30 * time.Second)
	log("Waiting for Dominion to respond…")
	for time.Now().Before(deadline) {
		url := fmt.Sprintf("http://localhost:%d/health", port)
		resp, err := http.Get(url) //nolint:noctx
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return port, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Read the log for diagnostics. Cap at 500 bytes from the tail so the error
	// message stays compact enough to display in the UI.
	logPath := filepath.Join(dominionRoot, "dominion.log")
	if logBytes, readErr := os.ReadFile(logPath); readErr == nil && len(logBytes) > 0 {
		preview := string(logBytes)
		const maxPreview = 500
		if len(preview) > maxPreview {
			preview = "…\n" + preview[len(preview)-maxPreview:]
		}
		return port, fmt.Errorf("dominion did not start within 30 seconds\n\nLog (%s):\n%s", logPath, preview)
	}
	return port, fmt.Errorf("dominion did not start within 30 seconds — check %s for details", logPath)
}

func findNode() (string, error) {
	// Check Volta first
	home, _ := os.UserHomeDir()
	var voltaNode string
	switch runtime.GOOS {
	case "windows":
		voltaNode = filepath.Join(os.Getenv("LOCALAPPDATA"), "Volta", "bin", "node.exe")
	default:
		voltaNode = filepath.Join(home, ".volta", "bin", "node")
	}
	if _, err := os.Stat(voltaNode); err == nil {
		return voltaNode, nil
	}

	// Fall back to PATH
	return exec.LookPath("node")
}

// registerLoginItem registers the Dominion as a user-level OS login item.
func registerLoginItem(mainJS string, dominionRoot string) error {
	nodePath, err := findNode()
	if err != nil {
		return fmt.Errorf("node not found for login item: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		return registerWindowsLoginItem(nodePath, mainJS, dominionRoot)
	case "darwin":
		return registerMacLaunchAgent(nodePath, mainJS, dominionRoot)
	default:
		return fmt.Errorf("login item registration not supported on %s", runtime.GOOS)
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func registerMacLaunchAgent(nodePath, mainJS, dominionRoot string) error {
	home, _ := os.UserHomeDir()
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return err
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.minions.dominion</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>%s</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>DOMINION_ROOT</key>
        <string>%s</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>%s/dominion.log</string>
    <key>StandardErrorPath</key>
    <string>%s/dominion-error.log</string>
</dict>
</plist>
`, nodePath, mainJS, dominionRoot, dominionRoot, dominionRoot)

	plistPath := filepath.Join(agentDir, "com.minions.dominion.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return err
	}

	// Load the launch agent
	return exec.Command("launchctl", "load", plistPath).Run()
}

// ensureNode checks for an existing Node.js installation and installs Volta
// (with Node.js LTS + pnpm) if one is not found.
func ensureNode(log func(string)) error {
	// Check if node is already available
	if _, err := exec.LookPath("node"); err == nil {
		return nil
	}

	log(fmt.Sprintf("Node.js not found. Installing Volta %s…", voltaVersion))

	switch runtime.GOOS {
	case "windows":
		return installNodeWindows(log)
	case "darwin":
		return installNodeMac(log)
	default:
		return fmt.Errorf("automatic Node.js installation not supported on %s; please install Node.js manually", runtime.GOOS)
	}
}

func installNodeWindows(log func(string)) error {
	// Use the zip release — same binary-extraction approach as macOS — so the
	// install runs entirely at user privilege with no msiexec / UAC prompt.
	url := fmt.Sprintf(
		"https://github.com/volta-cli/volta/releases/download/v%s/volta-%s-windows.zip",
		voltaVersion, voltaVersion,
	)
	voltaZip := filepath.Join(os.TempDir(), "volta-windows.zip")

	log(fmt.Sprintf("Downloading Volta %s for Windows…", voltaVersion))
	if err := downloadFile(url, voltaZip); err != nil {
		return fmt.Errorf("download volta: %w", err)
	}
	defer os.Remove(voltaZip)

	if voltaWindowsSHA256 != "" {
		log("Verifying Volta integrity…")
		if err := verifySHA256(voltaZip, voltaWindowsSHA256); err != nil {
			return fmt.Errorf("volta integrity check failed: %w", err)
		}
		log("Volta verified.")
	}

	// Extract volta.exe (and shim/migrate siblings) into %LOCALAPPDATA%\Volta\bin.
	voltaBinDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Volta", "bin")
	if err := os.MkdirAll(voltaBinDir, 0755); err != nil {
		return fmt.Errorf("create volta dir: %w", err)
	}
	log("Extracting Volta…")
	if err := extractZipFlat(voltaZip, voltaBinDir); err != nil {
		return fmt.Errorf("extract volta: %w", err)
	}

	// Add Volta bin to the user PATH (persistent, no admin required).
	_ = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`$p=[Environment]::GetEnvironmentVariable('PATH','User');`+
			`if($p -notlike '*\Volta\bin*'){`+
			`[Environment]::SetEnvironmentVariable('PATH',$p+';'+$env:LOCALAPPDATA+'\Volta\bin','User')`+
			`}`).Run()

	// Enable pnpm support in Volta.
	_ = exec.Command("setx", "VOLTA_FEATURE_PNPM", "1").Run()

	voltaBin := filepath.Join(voltaBinDir, "volta.exe")
	log("Installing Node.js LTS via Volta…")
	if err := exec.Command(voltaBin, "install", "node@lts").Run(); err != nil {
		return fmt.Errorf("volta install node: %w", err)
	}
	log("Installing pnpm via Volta…")
	if err := exec.Command(voltaBin, "install", "pnpm").Run(); err != nil {
		return fmt.Errorf("volta install pnpm: %w", err)
	}

	return nil
}

func installNodeMac(log func(string)) error {
	home, _ := os.UserHomeDir()

	// Select the right tarball for this CPU architecture.
	var tarName, sha string
	switch runtime.GOARCH {
	case "arm64":
		tarName = fmt.Sprintf("volta-%s-macos-aarch64.tar.gz", voltaVersion)
		sha = voltaMacARM64SHA256
	default:
		tarName = fmt.Sprintf("volta-%s-macos.tar.gz", voltaVersion)
		sha = voltaMacX86SHA256
	}

	url := fmt.Sprintf("https://github.com/volta-cli/volta/releases/download/v%s/%s", voltaVersion, tarName)
	tmp := filepath.Join(os.TempDir(), tarName)

	log(fmt.Sprintf("Downloading Volta %s for macOS (%s)…", voltaVersion, runtime.GOARCH))
	if err := downloadFile(url, tmp); err != nil {
		return fmt.Errorf("download volta: %w", err)
	}
	defer os.Remove(tmp)

	if sha != "" {
		log("Verifying Volta integrity…")
		if err := verifySHA256(tmp, sha); err != nil {
			return fmt.Errorf("volta integrity check failed: %w", err)
		}
		log("Volta verified.")
	}

	// Extract volta, volta-shim, volta-migrate into ~/.volta/bin
	voltaBinDir := filepath.Join(home, ".volta", "bin")
	if err := os.MkdirAll(voltaBinDir, 0755); err != nil {
		return fmt.Errorf("create volta dir: %w", err)
	}
	log("Extracting Volta…")
	if err := extractTarGz(tmp, voltaBinDir); err != nil {
		return fmt.Errorf("extract volta: %w", err)
	}

	voltaBin := filepath.Join(voltaBinDir, "volta")
	if _, err := os.Stat(voltaBin); err != nil {
		return fmt.Errorf("volta binary not found after extraction")
	}

	// Add Volta environment setup to shell profiles.
	profileLines := "\nexport VOLTA_HOME=\"$HOME/.volta\"\nexport PATH=\"$VOLTA_HOME/bin:$PATH\"\nexport VOLTA_FEATURE_PNPM=1\n"
	for _, pf := range []string{filepath.Join(home, ".bashrc"), filepath.Join(home, ".zshrc")} {
		if _, err := os.Stat(pf); err == nil {
			if f, err := os.OpenFile(pf, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
				_, _ = fmt.Fprint(f, profileLines)
				f.Close()
			}
		}
	}
	_ = os.Setenv("VOLTA_FEATURE_PNPM", "1")

	log("Installing Node.js LTS via Volta…")
	if err := exec.Command(voltaBin, "install", "node@lts").Run(); err != nil {
		return fmt.Errorf("volta install node: %w", err)
	}
	log("Installing pnpm via Volta…")
	if err := exec.Command(voltaBin, "install", "pnpm").Run(); err != nil {
		return fmt.Errorf("volta install pnpm: %w", err)
	}

	return nil
}

// extractTarGz extracts all regular files from a .tar.gz archive directly
// into destDir, flattening any directory structure in the archive.
func extractTarGz(src, destDir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Flatten paths: place all files directly in destDir.
		target := filepath.Join(destDir, filepath.Base(header.Name))
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, tr)
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// windowsRunKey is the registry key for user-level login items on Windows.
const windowsRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func registerWindowsLoginItem(nodePath, mainJS, dominionRoot string) error {
	value := fmt.Sprintf(`"%s" "%s"`, nodePath, mainJS)
	// Use reg.exe to set the registry value (user-level, no admin required)
	cmd := exec.Command("reg", "add",
		`HKCU\`+windowsRunKey,
		"/v", "Minions Dominion",
		"/t", "REG_SZ",
		"/d", value,
		"/f",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg add: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
