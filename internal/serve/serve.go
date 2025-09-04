package serve

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/netbrain/mnu/internal/util"
)

const advertiseSock = "serve.sock"

// Start launches `bw serve` on a random localhost port and waits for readiness.
// The returned cmd is owned by the caller and should be killed on exit.
func Start() (string, *exec.Cmd, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("failed to reserve a port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cmd := exec.Command("bw", "serve", "--port", strconv.Itoa(port))
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to start bw serve: %w", err)
	}

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitReady(apiURL, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return "", nil, err
	}
	return apiURL, cmd, nil
}

// FindAdvertised connects to the Unix domain socket (if any) to obtain the API URL.
func FindAdvertised() (string, bool) {
	configDir, err := util.GetConfigDir()
	if err != nil { return "", false }
	sock := filepath.Join(configDir, advertiseSock)
	conn, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
	if err != nil { return "", false }
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	b, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(b) == 0 { return "", false }
	return string(bytes.TrimSpace(b)), true
}

// RunAdvertiser starts `bw serve`, then listens on a Unix socket to advertise its URL.
// It blocks forever handling simple info requests from clients.
func RunAdvertiser() error {
	apiURL, cmd, err := Start()
	if err != nil { return err }
	defer func() { _ = cmd.Process.Kill() }()

	configDir, err := util.GetConfigDir()
	if err != nil { return err }
	sock := filepath.Join(configDir, advertiseSock)
	// Remove any stale socket
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil { return fmt.Errorf("failed to listen on socket: %w", err) }
	defer func() { ln.Close(); _ = os.Remove(sock) }()
	_ = os.Chmod(sock, 0600)

	for {
		conn, err := ln.Accept()
		if err != nil { return err }
		go func(c net.Conn) {
			defer c.Close()
			_, _ = c.Write([]byte(apiURL + "\n"))
		} (conn)
	}
}

func waitReady(apiURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(apiURL + "/status")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("bw serve did not become ready on %s within timeout", apiURL)
}

