package clipboard

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/netbrain/mnu/internal/util"
)

const uniqueIDFileName = "clipboard_clearer.id"

// Copy copies the given text and clears it after the given duration.
func Copy(text string, clearAfter time.Duration) error {
	b := []byte(text)
	defer func() {
		text = ""
		for i := range b { b[i] = 0 }
	}()
	return CopyBytes(b, clearAfter)
}

// CopyBytes copies the given bytes and clears them securely after the given duration.
func CopyBytes(content []byte, clearAfter time.Duration) error {
	configDir, err := util.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	uniqueIDFilePath := filepath.Join(configDir, uniqueIDFileName)

	// Cancel previous clearer if exists
	if _, err := os.Stat(uniqueIDFilePath); err == nil {
		if prevUniqueIDBytes, err := ioutil.ReadFile(uniqueIDFilePath); err == nil {
			prevUniqueID := string(prevUniqueIDBytes)
			fifoPath := filepath.Join(configDir, "clipboard_clearer_"+prevUniqueID+".fifo")
			if fifo, err := os.OpenFile(fifoPath, os.O_WRONLY|syscall.O_NONBLOCK, 0600); err == nil {
				fmt.Fprintf(fifo, "cancel")
				fifo.Close()
			}
		}
	}

	// New clearer
	newUniqueID := uuid.New().String()
	cmd := exec.Command(
		os.Args[0],
		"clear-clipboard",
		strconv.Itoa(int(clearAfter.Seconds())),
		newUniqueID,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe for clearer: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to start clipboard clearer process: %w", err)
	}
	if _, err := stdin.Write(content); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to write content to clearer stdin: %w", err)
	}
	stdin.Close()
	for i := range content { content[i] = 0 }

	if err := ioutil.WriteFile(uniqueIDFilePath, []byte(newUniqueID), 0644); err != nil {
		return fmt.Errorf("failed to write unique ID file: %w", err)
	}
	return nil
}

