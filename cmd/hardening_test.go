// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-knx/server"
	"github.com/otfabric/knxctl/internal/errs"
)

// Negative counts and durations were "no limit" (listen), "stop at once"
// (busmonitor, group answer exited 0 with nothing done) or ignored; they
// are invalid input now, rejected before connecting (a dead endpoint
// would time out with exit 5).
func TestNegativeLimitsRejected(t *testing.T) {
	t.Parallel()
	dead := deadPort(t)
	for _, args := range [][]string{
		{"busmonitor", dead, "--count", "-1"},
		{"busmonitor", dead, "--duration", "-1s"},
		{"group", "answer", "1/2/3", "on", "-g", dead, "--count", "-1"},
		{"group", "answer", "1/2/3", "on", "-g", dead, "--duration", "-1s"},
		{"group", "listen", "-g", dead, "--count", "-5"},
		{"monitor", "-g", dead, "--duration", "-1ms"},
		{"routing", "monitor", "--count", "-1"},
		{"group", "read", "1/2/3", "-g", dead, "--all", "-1s"},
		{"tunnel", "connect", dead, "--hold", "-1s"},
	} {
		r := run(t, append(args, "--timeout", "300ms", "--format", "json")...)
		want(t, r, 3)
		if e := errorOf(t, r); e["code"] != "invalid_input" {
			t.Fatalf("%v: %v", args, e)
		}
	}
}

// group answer --count -1 used to emit "answered 0" at once.
func TestAnswerNegativeCountSendsNothing(t *testing.T) {
	t.Parallel()
	s := startServer(t)
	want(t, run(t, "group", "answer", "1/2/3", "on", "-g", s.addr, "--count", "-1"), 3)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.telegrams) != 0 {
		t.Fatalf("%d telegrams sent", len(s.telegrams))
	}
}

func TestAddDuration(t *testing.T) {
	t.Parallel()
	const maxD = time.Duration(math.MaxInt64)
	for _, c := range []struct{ a, b, want time.Duration }{
		{time.Second, time.Second, 2 * time.Second},
		{maxD, time.Second, maxD},
		{maxD - time.Second, 2 * time.Second, maxD},
		{time.Second, 0, time.Second},
	} {
		if got := addDuration(c.a, c.b); got != c.want {
			t.Fatalf("addDuration(%v, %v) = %v", c.a, c.b, got)
		}
	}
}

// --timeout plus a margin wrapped to a negative deadline: the largest
// --timeout expired at once ("context deadline exceeded", exit 5).
func TestHugeTimeoutDoesNotWrap(t *testing.T) {
	t.Parallel()
	s := startServer(t, server.WithIndividualAddress(hostIA))
	const huge = "2562047h47m16s"
	want(t, run(t, "property", "read", s.addr, "device", "11", "--device", "1.1.0", "--timeout", huge), 0, "1.1.0 object 0x0000")
	want(t, run(t, "group", "read", "1/2/4", "--all", "200ms", "-g", s.addr, "--timeout", huge), 0, "0C33")
	want(t, run(t, "group", "read", "1/2/4", "--all", huge, "-g", s.addr, "--timeout", huge, "--dpt", "x"), 3)
}

// Oversized data was refused by go-knx only when sending: after
// connecting (exit 3 having connected; a local property write lost the
// connection, exit 4). It is invalid input before connecting now.
func TestOversizedDataRejectedBeforeConnecting(t *testing.T) {
	t.Parallel()
	dead := deadPort(t)
	hexN := func(n int) string { return strings.Repeat("A5", n) }
	for _, args := range [][]string{
		{"group", "write", "1/2/3", "--raw", hexN(maxGroupValue + 1), "-g", dead},
		{"group", "respond", "1/2/3", "--raw", hexN(maxGroupValue + 1), "-g", dead},
		{"group", "answer", "1/2/3", "--raw", hexN(maxGroupValue + 1), "-g", dead},
		{"property", "write", dead, "knxnetip", "76", hexN(maxLocalPropData + 1)},
		{"property", "write", dead, "knxnetip", "76", hexN(maxRemotePropData + 1), "--device", "1.1.0"},
		{"tunnel", "feature", "set", dead, "info-service", hexN(maxFeatureValue + 1)},
	} {
		r := run(t, append(args, "--timeout", "300ms", "--format", "json")...)
		want(t, r, 3)
		if e := errorOf(t, r); e["code"] != "invalid_value" {
			t.Fatalf("%v: %v", args, e)
		}
	}
	// The limits themselves pass validation (and then time out).
	for _, args := range [][]string{
		{"group", "write", "1/2/3", "--raw", hexN(maxGroupValue), "-g", dead},
		{"property", "write", dead, "knxnetip", "76", hexN(maxLocalPropData), "--count", "1"},
	} {
		want(t, run(t, append(args, "--timeout", "300ms")...), 5)
	}
}

// A local property write of the longest data reaches the server: the
// limit is not below what the frame holds.
func TestLocalPropertyWriteLimitFits(t *testing.T) {
	t.Parallel()
	s := startServer(t)
	r := run(t, "property", "write", s.addr, "knxnetip", "76", strings.Repeat("6B", maxLocalPropData), "--format", "json")
	if r.code == 4 || r.code == 3 {
		t.Fatalf("exit %d: the longest data did not fit\n%s", r.code, r.out)
	}
}

func TestDecodeHex(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"0x0C33": "\x0c\x33", "0X0c33": "\x0c\x33", "0C 33": "\x0c\x33", "": ""} {
		b, err := decodeHex(in)
		if err != nil || string(b) != want {
			t.Fatalf("%q: %x %v", in, b, err)
		}
	}
	for _, in := range []string{"0x", "0xZZ", "x0C", "0C3"} {
		if b, err := decodeHex(in); err == nil && len(b) > 0 {
			t.Fatalf("%q decoded to %x", in, b)
		}
	}
	want(t, run(t, "dpt", "decode", "9.001", "0X0C33"), 0, "21.5")
	want(t, run(t, "decode", "0X06100421000a04010000"), 0, "TUNNELLING_ACK")
}

// dpt list --main of a number no type has could never match: an empty
// list, exit 0.
func TestDPTListMainRange(t *testing.T) {
	t.Parallel()
	want(t, run(t, "dpt", "list", "--main", "-1"), 3)
	want(t, run(t, "dpt", "list", "--main", "65536"), 3)
	want(t, run(t, "dpt", "list", "--main", "9"), 0, "9.001")
}

// A keyring or configuration that is not a small file (a device, a FIFO
// fed forever) was read whole: unbounded memory, or a hang.
func TestInputFilesBounded(t *testing.T) {
	t.Parallel()
	big := filepath.Join(t.TempDir(), "big")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxKeyringSize + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	paths := []string{big}
	if runtime.GOOS != "windows" {
		paths = append(paths, "/dev/zero") // endless
	}
	for _, p := range paths {
		t0 := time.Now()
		want(t, run(t, "keyring", "show", p, "--keyring-password", "x"), 3, "larger than")
		want(t, run(t, "serve", "--config", p, "--listen", "127.0.0.1:0"), 3, "larger than")
		want(t, run(t, "group", "write", "1/2/3", "on", "-g", "127.0.0.1:9", "--keyring", p), 3, "larger than")
		if d := time.Since(t0); d > 10*time.Second {
			t.Fatalf("%s: %v", p, d)
		}
	}
	if _, err := readFileLimited(filepath.Join(t.TempDir(), "missing"), 10); err == nil {
		t.Fatal("a missing file read")
	}
	if _, err := readFileLimited(t.TempDir(), 10); err == nil {
		t.Fatal("a directory read")
	}
}

// diagnose checked its credentials after talking to the server: a bad
// --user-id ran the search and description first (exit 1 when they
// failed) instead of exit 3.
func TestDiagnoseCredentialsFirst(t *testing.T) {
	t.Parallel()
	r := run(t, "diagnose", deadPort(t), "--user-id", "200", "--timeout", "300ms", "--format", "json")
	want(t, r, 3)
	errorOf(t, r)
}

// Ctrl-C during a wait that go-knx ends with "no answer" (a timeout)
// exited 5; it is an interruption, 130.
func TestInterruptedIsNotATimeout(t *testing.T) {
	t.Parallel()
	s := startServer(t, server.WithIndividualAddress(hostIA))
	// 1.1.77 does not exist: the command waits for its answer.
	b := start(t, "property", "read", s.addr, "device", "11", "--device", "1.1.77", "--timeout", "20s", "--format", "json", "-v")
	eventually(t, "the tunnel", func() bool { return len(s.Connections()) > 0 })
	time.Sleep(200 * time.Millisecond)
	r := b.stop(t)
	want(t, r, 130)
	if e := errorOf(t, r); e["kind"] != "interrupted" || e["code"] != "canceled" {
		t.Fatalf("%v", e)
	}
	// Input errors stay input errors.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := errs.Invalid("bad")
	if e := interrupted(ctx, in); e != in {
		t.Fatalf("%v", e)
	}
	timeout := &errs.Error{Kind: errs.KindTimeout, Code: errs.CodeTimeout, Message: "late"}
	if e := interrupted(context.Background(), timeout); e != timeout {
		t.Fatalf("%v", e)
	}
	// A timeout of a cancelled command is the interruption.
	if e := interrupted(ctx, timeout); e.Kind != errs.KindInterrupted || e.Code != errs.CodeCanceled || e.Cause != timeout {
		t.Fatalf("cancelled timeout: %+v", e)
	}
}

// A stream that fails after its records ended with an indented error
// document: the output was no longer JSON Lines.
func TestStreamErrorIsOneJSONLine(t *testing.T) {
	t.Parallel()
	s := startServer(t)
	b := start(t, "monitor", "-g", s.addr, "--format", "json")
	b.waitErr(t, "listening")
	want(t, run(t, "group", "write", "1/2/3", "on", "-g", s.addr), 0)
	eventually(t, "a record", func() bool { return strings.Contains(b.out.String(), `"telegram"`) })
	_ = s.Close()
	r := b.wait(t)
	want(t, r, 4)
	lines := jsonLines(t, r.out)
	if len(lines) < 2 || lines[len(lines)-1]["error"] == nil {
		t.Fatalf("no error line at the end\n%s", r.out)
	}
}
