package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DominionConfig is persisted in the OS app data directory.
type DominionConfig struct {
	Root string `json:"root"`
}

func appDataDir() string {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, _ := os.UserHomeDir()
			appData = home
		}
		return filepath.Join(appData, "Minions")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Minions")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "Minions")
	}
}

func persistDominionConfig(root string) error {
	dir := appDataDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create app data dir: %w", err)
	}
	configFile := filepath.Join(dir, "dominion.json")
	config := DominionConfig{Root: root}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
}

// readDominionConfig reads the previously saved install location.
// Returns an error if no config exists yet.
func readDominionConfig() (string, error) {
	configPath := filepath.Join(appDataDir(), "dominion.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var config DominionConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}
	if config.Root == "" {
		return "", fmt.Errorf("root not found in config")
	}
	return config.Root, nil
}
