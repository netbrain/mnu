package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	uipkg "github.com/netbrain/bwmenu/internal/ui"
	runner "github.com/netbrain/bwmenu/internal/runner"
	bwpkg "github.com/netbrain/bwmenu/internal/bw"

	cfgpkg "github.com/netbrain/bwmenu/internal/config"
	"github.com/netbrain/bwmenu/internal/debugflag"
	"github.com/netbrain/bwmenu/internal/keychain"
	"github.com/netbrain/bwmenu/internal/serve"
	"github.com/netbrain/bwmenu/internal/util"
)

var bwManager bwpkg.Manager
var debug bool

// This is the new subcommand logic
func clearClipboardSubcommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: bwmenu clear-clipboard <timeout_seconds> <unique_id> (content via stdin)")
		os.Exit(1)
	}

	// Read content from stdin to avoid exposing secrets in process args
	contentBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Printf("Failed to read content from stdin: %v\n", err)
		os.Exit(1)
	}
	// Compute hash of original content for sanity check and then clear buffers later
	origHash := sha256.Sum256(contentBytes)
	content := string(contentBytes)
	// Zero the input bytes ASAP after constructing the string copy
	for i := range contentBytes {
		contentBytes[i] = 0
	}

	timeoutSeconds, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Printf("Invalid timeout: %v\n", err)
		os.Exit(1)
	}
	uniqueID := os.Args[2]

	configDir, err := util.GetConfigDir()
	if err != nil {
		fmt.Printf("Failed to get config directory: %v\n", err)
		os.Exit(1)
	}
	fifoPath := filepath.Join(configDir, "clipboard_clearer_"+uniqueID+".fifo")

	// Create the named pipe
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		fmt.Printf("Failed to create FIFO: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(fifoPath) // Clean up the FIFO on exit

	// Copy content to clipboard
	if err := clipboard.WriteAll(content); err != nil {
		fmt.Printf("Failed to copy to clipboard: %v\n", err)
		os.Exit(1)
	}
	// Immediately drop the content string reference to reduce exposure window
	content = ""

	cancelChan := make(chan struct{})
	go func() {
		// Open the FIFO for reading (blocks until a writer connects)
		fifo, err := os.OpenFile(fifoPath, os.O_RDONLY, 0600)
		if err != nil {
			fmt.Printf("Failed to open FIFO for reading: %v\n", err)
			return
		}
		defer fifo.Close()

		// Read from the FIFO (blocks until data is written)
		buffer := make([]byte, 1)
		_, err = fifo.Read(buffer)
		if err != nil && err != io.EOF {
			fmt.Printf("Error reading from FIFO: %v\n", err)
			return
		}
		close(cancelChan) // Signal cancellation
	}()

	select {
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		// Timeout reached, clear clipboard only if clipboard still contains our original content
		current, err := clipboard.ReadAll()
		if err != nil {
			fmt.Printf("Failed to read clipboard for sanity check: %v\n", err)
			os.Exit(1)
		}
		curHash := sha256.Sum256([]byte(current))
		if curHash != origHash {
			// Clipboard changed; do not clear
			fmt.Println("Clipboard content changed; skipping clear.")
			return
		}
		if err := clipboard.WriteAll(""); err != nil {
			fmt.Printf("Failed to clear clipboard: %v\n", err)
			os.Exit(1)
		}
	case <-cancelChan:
		// Cancel signal received, do not clear clipboard
		fmt.Println("Clipboard clearer cancelled.")
	}
}

func serveSubcommand() {
	// If already advertised, do nothing.
	if url, ok := serve.FindAdvertised(); ok {
		fmt.Printf("bw serve already running at %s\n", url)
		return
	}
	if err := serve.RunAdvertiser(); err != nil {
		fmt.Printf("Failed to run bw serve advertiser: %v\n", err)
		os.Exit(1)
	}
}

func appsMain() {
	// single-instance guard for the runner
	lockFile, err := util.AcquireNamedLock("bwmenu-apps.lock")
	if err != nil {
		fmt.Println("bwmenu apps is already running; exiting.")
		os.Exit(0)
	}
	defer util.ReleaseAppLock(lockFile)

	p := tea.NewProgram(runner.InitialModel())
	m, err := p.Run()
	if err != nil {
		fmt.Printf("apps: %v\n", err)
		os.Exit(1)
	}
	if rm, ok := m.(runner.Model); ok {
		if rm.SelectedPath() != "" {
			// Launch the selected program as a detached background process and exit bwmenu
			devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
			if err != nil {
				fmt.Printf("failed to open %s: %v\n", os.DevNull, err)
				os.Exit(1)
			}
			defer devNull.Close()

			// Detect Flatpak exported wrappers and run via `flatpak run <appID>`
			launchPath := rm.SelectedPath()
			var cmd *exec.Cmd
			if strings.Contains(launchPath, "/flatpak/exports/bin/") {
				appID := filepath.Base(launchPath)
				cmd = exec.Command("flatpak", "run", appID)
			} else {
				cmd = exec.Command(launchPath)
			}
			cmd.Stdin = devNull
			cmd.Stdout = devNull
			cmd.Stderr = devNull
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid: true, // start a new session (detach from controlling terminal)
			}
			if err := cmd.Start(); err != nil {
				fmt.Printf("failed to start %s: %v\n", launchPath, err)
				os.Exit(1)
			}
			// Do not wait; exit bwmenu immediately after spawning
			return
		}
	}
}

func main() {
	// Check for subcommands that bypass the single-instance guard
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "clear-clipboard":
			os.Args = os.Args[1:]
			clearClipboardSubcommand()
			return
		case "serve":
			serveSubcommand()
			return
		case "apps":
			appsMain()
			return
		}
	}

	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()

	// Single-instance guard for the TUI
	lockFile, err := util.AcquireAppLock()
	if err != nil {
		fmt.Println("bwmenu is already running; exiting.")
		os.Exit(0)
	}
	defer util.ReleaseAppLock(lockFile)

	if debug {
		debugflag.Enabled = true
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}
		defer f.Close()
		log.Println("Debug is enabled")
	}

	config, err := cfgpkg.Load()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}

	// Check for existing session key and set BW_SESSION if found
	sessionKey, err := keychain.GetSessionKey()
	if err == nil && sessionKey != "" {
		os.Setenv("BW_SESSION", sessionKey)
	}

	var bwServeCmd *exec.Cmd
	if config.ApiMode {
		if apiUrl, ok := serve.FindAdvertised(); ok {
			bwManager = bwpkg.NewAPIManager(apiUrl)
		} else {
			apiUrl, cmd, err := serve.Start()
			if err != nil {
				fmt.Printf("Alas, there's been an error: %v\n", err)
				os.Exit(1)
			}
			bwServeCmd = cmd
			bwManager = bwpkg.NewAPIManager(apiUrl)
		}
	} else {
		bwManager = bwpkg.NewProcessManager()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		if bwServeCmd != nil {
			bwServeCmd.Process.Kill()
		}
		os.Exit(0)
	}()

	p := tea.NewProgram(uipkg.InitialModel(bwManager, config))

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}

	if bwServeCmd != nil {
		bwServeCmd.Process.Kill()
	}
}
