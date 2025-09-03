package serve

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// Start launches `bw serve` on a random localhost port and waits for readiness.
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
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(apiURL + "/status")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return apiURL, cmd, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return "", nil, fmt.Errorf("bw serve did not become ready on %s within timeout", apiURL)
}

