// SPDX-License-Identifier: MIT

//go:build e2e && !windows

package e2e

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// readyBuffer is written by the process copier and read by the test;
// ready is closed once it holds marker.
type readyBuffer struct {
	mu     sync.Mutex
	b      bytes.Buffer
	marker string
	ready  chan struct{}
	once   sync.Once
}

func newReadyBuffer(marker string) *readyBuffer {
	return &readyBuffer{marker: marker, ready: make(chan struct{})}
}

func (l *readyBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, err := l.b.Write(p)
	if strings.Contains(l.b.String(), l.marker) {
		l.once.Do(func() { close(l.ready) })
	}
	return n, err
}

// silentPeer is a UDP endpoint on 127.0.0.1 that answers nothing; heard is
// closed when its first datagram arrives.
func silentPeer(t *testing.T) (addr string, heard <-chan struct{}) {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ch := make(chan struct{})
	go func() {
		buf := make([]byte, 1500)
		if _, _, err := c.ReadFromUDP(buf); err == nil {
			close(ch)
		}
	}()
	return c.LocalAddr().String(), ch
}

// interrupt starts the binary with stderr going to errOut, sends SIGINT
// once ready is closed, and returns its exit code and stdout. A binary
// that is not ready within 10 s fails the test.
func interrupt(t *testing.T, ready <-chan struct{}, errOut io.Writer, args ...string) (int, string) {
	t.Helper()
	bin := os.Getenv("KNXCTL_E2E_BIN")
	if bin == "" {
		t.Fatal("KNXCTL_E2E_BIN is not set; run make e2e")
	}
	var out bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = &out, errOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("%v ended before it was ready: %v\n%s", args, err, out.String())
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%v not ready within 10 s", args)
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
// JSON error document (docs/exitcodes.md); a stream ends with 0. Each
// command is interrupted once its first datagram has reached its own
// silent peer, so it is running and waiting.
func TestSIGINT(t *testing.T) {
	for _, args := range []func(addr string) []string{
		func(addr string) []string { return []string{"group", "read", "1/2/3", "-g", addr} },
		func(addr string) []string { return []string{"describe", addr} },
		func(addr string) []string { return []string{"property", "read", addr, "device", "11"} },
	} {
		addr, heard := silentPeer(t)
		a := append(args(addr), "--timeout", "30s", "--format", "json")
		code, out := interrupt(t, heard, io.Discard, a...)
		if code != 130 || !strings.Contains(out, `"interrupted"`) {
			t.Fatalf("%v: exit %d\n%s", a, code, out)
		}
	}
	errOut := newReadyBuffer("serving on")
	if code, out := interrupt(t, errOut.ready, errOut, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--no-tcp"); code != 0 {
		t.Fatalf("serve: exit %d\n%s", code, out)
	}
}
