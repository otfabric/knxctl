// SPDX-License-Identifier: MIT

//go:build e2e && !windows

package e2e

import (
	"bytes"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// lockedBuffer is written by the process copier and read by the test.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// interrupt starts the binary, sends SIGINT once stderr holds ready (or
// after a pause when ready is ""), and returns its exit code and stdout.
func interrupt(t *testing.T, ready string, args ...string) (int, string) {
	t.Helper()
	bin := os.Getenv("KNXCTL_E2E_BIN")
	if bin == "" {
		t.Fatal("KNXCTL_E2E_BIN is not set; run make e2e")
	}
	var out bytes.Buffer
	errOut := &lockedBuffer{}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = &out, errOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	time.Sleep(300 * time.Millisecond)
	for ready != "" && !strings.Contains(errOut.String(), ready) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		var exit *exec.ExitError
		switch {
		case err == nil:
			return 0, out.String()
		case errors.As(err, &exit):
			return exit.ExitCode(), out.String()
		default:
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v did not end on SIGINT", args)
	}
	return -1, ""
}

// SIGINT while a command waits for a server ends it with 130 and one
// JSON error document (docs/exitcodes.md); a stream ends with 0.
func TestSIGINT(t *testing.T) {
	silent, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	addr := silent.LocalAddr().String()
	for _, args := range [][]string{
		{"group", "read", "1/2/3", "-g", addr},
		{"describe", addr},
		{"property", "read", addr, "device", "11"},
	} {
		code, out := interrupt(t, "", append(args, "--timeout", "30s", "--format", "json")...)
		if code != 130 || !strings.Contains(out, `"interrupted"`) {
			t.Fatalf("%v: exit %d\n%s", args, code, out)
		}
	}
	if code, out := interrupt(t, "serving on", "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--no-tcp"); code != 0 {
		t.Fatalf("serve: exit %d\n%s", code, out)
	}
}
