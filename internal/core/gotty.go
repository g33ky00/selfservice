package core

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type gottyConfig struct {
	Bin    string
	Host   string
	LogDir string
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}

func waitPort(port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("gotty: port %d not ready after %s", port, timeout)
}

// startGotty launches gotty for SSH-in-browser access.
// sshArgs are passed verbatim after `ssh` (e.g. ["-p", "2222", "user@host"]).
func startGotty(cfg gottyConfig, sshArgs []string, token string) (*os.Process, int, error) {
	port, err := freePort()
	if err != nil {
		return nil, 0, fmt.Errorf("gotty: pick port: %w", err)
	}
	if err := os.MkdirAll(cfg.LogDir, 0700); err != nil {
		return nil, 0, fmt.Errorf("gotty: mkdir logs: %w", err)
	}
	logPath := filepath.Join(cfg.LogDir, "gotty_"+token+".log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, 0, fmt.Errorf("gotty: log file: %w", err)
	}
	defer lf.Close()

	args := []string{
		"-w", "-r",
		"--ws-origin", cfg.Host,
		"--port", fmt.Sprintf("%d", port),
		"--address", "127.0.0.1",
		"-t", "xterm-256color",
		"--permit-write",
		"ssh",
	}
	args = append(args, sshArgs...)

	cmd := exec.Command(cfg.Bin, args...)
	cmd.Stdout = lf
	cmd.Stderr = lf
	if err := cmd.Start(); err != nil {
		return nil, 0, fmt.Errorf("gotty start: %w", err)
	}
	if err := waitPort(port, 5*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return nil, 0, err
	}
	return cmd.Process, port, nil
}
