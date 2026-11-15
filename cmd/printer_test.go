// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// stalledStdout takes no output until it is released, as a pager that
// stopped reading: Write blocks until then.
type stalledStdout struct {
	mu       sync.Mutex
	b        bytes.Buffer
	released chan struct{}
	writing  chan struct{} // closed at the first Write
	wrote    chan struct{} // one value per Write done
	first    sync.Once
	release1 sync.Once
}

func newStalledStdout(t *testing.T) *stalledStdout {
	w := &stalledStdout{released: make(chan struct{}), writing: make(chan struct{}), wrote: make(chan struct{}, 64)}
	t.Cleanup(w.release)
	return w
}

func (w *stalledStdout) Write(p []byte) (int, error) {
	w.first.Do(func() { close(w.writing) })
	<-w.released
	w.mu.Lock()
	n, err := w.b.Write(p)
	w.mu.Unlock()
	w.wrote <- struct{}{}
	return n, err
}

func (w *stalledStdout) release() { w.release1.Do(func() { close(w.released) }) }

func (w *stalledStdout) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// serving is a knxctl serve running in process with the given stdout.
type serving struct {
	addr   string
	errOut *signalBuffer
	done   chan int
	cancel context.CancelFunc
}

func serveTo(t *testing.T, out io.Writer, args ...string) *serving {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &serving{errOut: newSignalBuffer("serving on "), done: make(chan int, 1), cancel: cancel}
	go func() {
		s.done <- Run(ctx, append([]string{"serve", "--listen", "127.0.0.1:0", "--no-discovery"}, args...), strings.NewReader(""), out, s.errOut)
	}()
	t.Cleanup(cancel)
	await(t, s.errOut.ready, "serving on")
	m := servingRe.FindStringSubmatch(s.errOut.String())
	if m == nil {
		t.Fatalf("no address in %q", s.errOut.String())
	}
	s.addr = m[1]
	return s
}

// stop interrupts serve (Ctrl-C) and returns its exit code and stderr.
func (s *serving) stop(t *testing.T) (int, string) {
	t.Helper()
	s.cancel()
	select {
	case code := <-s.done:
		return code, s.errOut.String()
	case <-time.After(20 * time.Second):
		t.Fatalf("serve did not end after Ctrl-C\nstderr: %s", s.errOut.String())
	}
	return 0, ""
}

// setServeOutput sets the queue and the drain timeout of serve's output
// for one test, which must not run in parallel.
func setServeOutput(t *testing.T, queue int, drain time.Duration) {
	q, d := serveQueue, serveDrainTimeout
	serveQueue, serveDrainTimeout = queue, drain
	t.Cleanup(func() { serveQueue, serveDrainTimeout = q, d })
}

// writeValue writes n (DPT 5.010) to 1/2/3 through a tunnel of serve.
func writeValue(t *testing.T, addr string, n int) {
	t.Helper()
	want(t, run(t, "group", "write", "1/2/3", strconv.Itoa(n), "--dpt", "5.010", "-g", addr), 0)
}

// A stdout that does not take the output holds up neither the server nor
// its clients: the telegrams beyond the queue are dropped and counted,
// the next record carries the count, and Ctrl-C reports it.
func TestServeStdoutThatDoesNotRead(t *testing.T) {
	setServeOutput(t, 3, 10*time.Second)
	w := newStalledStdout(t)
	s := serveTo(t, w, "--format", "json")
	writeValue(t, s.addr, 1)
	await(t, w.writing, "the first record") // record 1 is being written
	for n := 2; n <= 6; n++ {
		writeValue(t, s.addr, n) // 2, 3 and 4 are queued; 5 and 6 are dropped
	}
	want(t, run(t, "describe", s.addr), 0)
	w.release()
	for range 4 {
		await(t, w.wrote, "the queued records")
	}
	writeValue(t, s.addr, 7)
	await(t, w.wrote, "the record after the dropped ones")
	code, errOut := s.stop(t)
	if code != 0 || !strings.Contains(errOut, "knxctl: 2 telegrams dropped (output too slow)") {
		t.Fatalf("exit %d\nstderr: %s", code, errOut)
	}
	var data []string
	for _, r := range jsonLines(t, strings.ReplaceAll(w.String(), "\r\n", "\n")) {
		data = append(data, r["data"].(string))
		if d, ok := r["dropped"]; ok != (r["data"] == "07") || (ok && d != 2.0) {
			t.Errorf("record %v: dropped %v", r["data"], d)
		}
	}
	if strings.Join(data, " ") != "01 02 03 04 07" {
		t.Fatalf("records %v", data)
	}
}

// Ctrl-C ends serve within the drain timeout while stdout takes nothing;
// the records not written are counted.
func TestServeEndsWhileStdoutBlocks(t *testing.T) {
	setServeOutput(t, 1024, 100*time.Millisecond)
	w := newStalledStdout(t)
	s := serveTo(t, w)
	writeValue(t, s.addr, 1)
	await(t, w.writing, "the first record")
	writeValue(t, s.addr, 2)
	code, errOut := s.stop(t)
	if code != 0 || !strings.Contains(errOut, "knxctl: 2 telegrams dropped (output too slow)") {
		t.Fatalf("exit %d\nstderr: %s", code, errOut)
	}
}

// The text line of a record after dropped telegrams says how many.
func TestTelegramLineDropped(t *testing.T) {
	r := telegramRecord{Time: "t", Source: "1.1.1", Group: "1/2/3", Service: "write", Data: "01", Dropped: 2}
	if l := r.line(); !strings.HasSuffix(l, " 01 [2 dropped before]") {
		t.Fatalf("line %q", l)
	}
	r.Dropped = 0
	if l := r.line(); strings.Contains(l, "dropped") {
		t.Fatalf("line %q", l)
	}
}
