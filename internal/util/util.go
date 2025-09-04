package util

import (
	"os"
	"path/filepath"
	"syscall"
)

// GetConfigDir returns ~/.config/mnu, creating it if necessary.
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "mnu")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}
	return configDir, nil
}

// AcquireAppLock attempts to acquire an exclusive, non-blocking lock on a lock file
// in the mnu config directory. It returns the open file handle which must be kept
// open for the lifetime of the process to hold the lock. Call ReleaseAppLock to unlock.
func AcquireAppLock() (*os.File, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(configDir, "mnu.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// AcquireNamedLock acquires an exclusive, non-blocking lock on the given lock file
// name under the mnu config directory. The returned *os.File must be kept open
// to hold the lock and closed (via ReleaseAppLock) to release it.
func AcquireNamedLock(lockFileName string) (*os.File, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(configDir, lockFileName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// ReleaseAppLock releases the lock acquired by AcquireAppLock.
func ReleaseAppLock(f *os.File) error {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return f.Close()
}
