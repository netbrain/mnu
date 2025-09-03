package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
	"syscall"

	"github.com/google/uuid"
	"github.com/netbrain/bwmenu/internal/util"
)

const uniqueIDFileName = "clipboard_clearer.id"

// Copy the given text to the clipboard and clear it after a given duration.
func copyToClipboard(text string, clearAfter time.Duration) error {
	// For compatibility, convert to []byte and call the secure variant.
	b := []byte(text)
	defer func() {
		// Best effort: clear the original string reference and zero bytes.
		text = ""
		for i := range b { b[i] = 0 }
	}()
	return copyToClipboardBytes(b, clearAfter)
}

// Secure variant that accepts bytes and zeroes them after use.
func copyToClipboardBytes(content []byte, clearAfter time.Duration) error {
	// Get the path for the unique ID file
	configDir, err := util.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	uniqueIDFilePath := filepath.Join(configDir, uniqueIDFileName)

	// Read previous unique ID if it exists and send cancel signal
	if _, err := os.Stat(uniqueIDFilePath); err == nil {
		prevUniqueIDBytes, err := ioutil.ReadFile(uniqueIDFilePath)
		if err == nil {
			prevUniqueID := string(prevUniqueIDBytes)
			fifoPath := filepath.Join(configDir, "clipboard_clearer_"+prevUniqueID+".fifo")
			// Open the FIFO for writing (non-blocking) and send a cancel signal
			if fifo, err := os.OpenFile(fifoPath, os.O_WRONLY|syscall.O_NONBLOCK, 0600); err == nil {
				fmt.Fprintf(fifo, "cancel")
				fifo.Close()
			}
		}
	}

	// Generate a new unique ID
	newUniqueID := uuid.New().String()

	// Launch bwmenu clear-clipboard as a separate, detached process
	cmd := exec.Command(
		os.Args[0], // Path to the current executable (bwmenu)
		"clear-clipboard",
		strconv.Itoa(int(clearAfter.Seconds())),
		newUniqueID, // Pass the unique ID to the clearer process
	)
	// Detach child so it survives parent crashes/exit
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe for clearer: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to start clipboard clearer process: %w", err)
	}

	// Write content to child's stdin, then zero our buffer
	if _, err := stdin.Write(content); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to write content to clearer stdin: %w", err)
	}
	stdin.Close()

	for i := range content {
		content[i] = 0
	}

	// Write new unique ID to file
	if err := ioutil.WriteFile(uniqueIDFilePath, []byte(newUniqueID), 0644); err != nil {
		return fmt.Errorf("failed to write unique ID file: %w", err)
	}

	return nil
}


