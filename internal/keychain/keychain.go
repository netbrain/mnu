package keychain

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const serviceName = "mnu"

func SetSessionKey(password string) error {
	user := os.Getenv("USER")
	if user == "" {
		return fmt.Errorf("USER environment variable not set")
	}
	err := keyring.Set(serviceName, user, password)
	if err != nil {
		// fall back to file
		path, e := getSessionKeyPath()
		if e != nil {
			return e
		}
		return os.WriteFile(path, []byte(password), 0600)
	}
	return nil
}

var ErrSessionKeyNotFound = fmt.Errorf("session key not found")

func GetSessionKey() (string, error) {
	user := os.Getenv("USER")
	if user == "" {
		return "", fmt.Errorf("USER environment variable not set")
	}
	password, err := keyring.Get(serviceName, user)
	if err != nil {
		path, e := getSessionKeyPath()
		if e != nil {
			return "", e
		}
		data, e := os.ReadFile(path)
		if e != nil {
			if os.IsNotExist(e) {
				return "", ErrSessionKeyNotFound
			}
			return "", e
		}
		if len(data) == 0 {
			return "", ErrSessionKeyNotFound
		}
		return string(data), nil
	}
	return password, nil
}

func DeleteSessionKey() error {
	user := os.Getenv("USER")
	if user == "" {
		return fmt.Errorf("USER environment variable not set")
	}
	err := keyring.Delete(serviceName, user)
	if err != nil {
		log.Printf("Warning: could not delete session key from keyring: %v", err)
	}

	path, err := getSessionKeyPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		err = os.Remove(path)
		if err != nil {
			return err
		}
	}

	return nil
}

func getSessionKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "mnu")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(configDir, "session"), nil
}
