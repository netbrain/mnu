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

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	bwpkg "github.com/netbrain/mnu/internal/bw"
	cfgpkg "github.com/netbrain/mnu/internal/config"
	"github.com/netbrain/mnu/internal/debugflag"
	"github.com/netbrain/mnu/internal/keychain"
	"github.com/netbrain/mnu/internal/serve"
	uipkg "github.com/netbrain/mnu/internal/ui"
	"github.com/netbrain/mnu/internal/util"
)

var bwManager bwpkg.Manager
var debug bool

func clearClipboardSubcommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: mnu-bw clear-clipboard <timeout_seconds> <unique_id> (content via stdin)")
		os.Exit(1)
	}

	contentBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Printf("Failed to read content from stdin: %v\n", err)
		os.Exit(1)
	}
	origHash := sha256.Sum256(contentBytes)
	content := string(contentBytes)
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

	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		fmt.Printf("Failed to create FIFO: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(fifoPath)

	if err := clipboard.WriteAll(content); err != nil {
		fmt.Printf("Failed to copy to clipboard: %v\n", err)
		os.Exit(1)
	}
	content = ""

	cancelChan := make(chan struct{})
	go func() {
		fifo, err := os.OpenFile(fifoPath, os.O_RDONLY, 0600)
		if err != nil {
			fmt.Printf("Failed to open FIFO for reading: %v\n", err)
			return
		}
		defer fifo.Close()
		buf := make([]byte, 1)
		_, err = fifo.Read(buf)
		if err != nil && err != io.EOF {
			fmt.Printf("Error reading from FIFO: %v\n", err)
			return
		}
		close(cancelChan)
	}()

	select {
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		current, err := clipboard.ReadAll()
		if err != nil {
			fmt.Printf("Failed to read clipboard for sanity check: %v\n", err)
			os.Exit(1)
		}
		curHash := sha256.Sum256([]byte(current))
		if curHash != origHash {
			fmt.Println("Clipboard content changed; skipping clear.")
			return
		}
		if err := clipboard.WriteAll(""); err != nil {
			fmt.Printf("Failed to clear clipboard: %v\n", err)
			os.Exit(1)
		}
	case <-cancelChan:
		fmt.Println("Clipboard clearer cancelled.")
	}
}

func serveSubcommand() {
	if url, ok := serve.FindAdvertised(); ok {
		fmt.Printf("bw serve already running at %s\n", url)
		return
	}
	if err := serve.RunAdvertiser(); err != nil {
		fmt.Printf("Failed to run bw serve advertiser: %v\n", err)
		os.Exit(1)
	}
}

func bitwardenMain() {
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()

	lockFile, err := util.AcquireAppLock()
	if err != nil {
		fmt.Println("mnu-bw is already running; exiting.")
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

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "clear-clipboard":
			os.Args = os.Args[1:]
			clearClipboardSubcommand()
			return
		case "serve":
			serveSubcommand()
			return
		}
	}
	bitwardenMain()
}
