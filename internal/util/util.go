package util

import (
	"os"
	"path/filepath"
)

// GetConfigDir returns ~/.config/bwmenu, creating it if necessary.
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "bwmenu")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}
	return configDir, nil
}

