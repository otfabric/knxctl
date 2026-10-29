// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/client"
	"github.com/otfabric/go-knx/device"
	"github.com/otfabric/go-knx/routing"
	"github.com/otfabric/go-knx/server"
)

func TestMain(m *testing.M) {
	// The environment must not choose formats or gateways for the tests.
	_ = os.Unsetenv("KNXCTL_FORMAT")
	_ = os.Unsetenv("KNXCTL_GATEWAY")
	os.Exit(m.Run())
}

type result struct {
	code        int
	out, errOut string
}

// syncBuffer is a bytes.Buffer safe for a command writing in one
// goroutine and a test reading in another.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// run executes knxctl in process.
func run(t *testing.T, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var out, errOut bytes.Buffer
	code := Run(ctx, args, strings.NewReader(""), &out, &errOut)
	return result{code, out.String(), errOut.String()}
}

// background runs knxctl in a goroutine; wait returns its result.
type background struct {
	out, errOut *syncBuffer
	done        chan int
	cancel      context.CancelFunc
}

func start(t *testing.T, args ...string) *background {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	b := &background{out: &syncBuffer{}, errOut: &syncBuffer{}, done: make(chan int, 1), cancel: cancel}
	go func() { b.done <- Run(ctx, args, strings.NewReader(""), b.out, b.errOut) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-b.done:
		case <-time.After(10 * time.Second):
		}
	})
	return b
}

// waitErr waits until stderr contains s.
func (b *background) waitErr(t *testing.T, s string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(b.errOut.String(), s) {
		select {
		case code := <-b.done:
			b.done <- code
			t.Fatalf("command ended (exit %d) before %q\nstdout: %s\nstderr: %s", code, s, b.out.String(), b.errOut.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q\nstderr: %s", s, b.errOut.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// wait returns the result once the command ends.
func (b *background) wait(t *testing.T) result {
	t.Helper()
	select {
	case code := <-b.done:
		b.done <- code
		return result{code, b.out.String(), b.errOut.String()}
	case <-time.After(20 * time.Second):
		t.Fatalf("command did not end\nstdout: %s\nstderr: %s", b.out.String(), b.errOut.String())
	}
	return result{}
}

// ended reports whether the command has ended.
func (b *background) ended() bool {
	select {
	case code := <-b.done:
		b.done <- code
		return true
	default:
		return false
	}
}

// stop interrupts the command (Ctrl-C) and returns its result.
func (b *background) stop(t *testing.T) result {
	t.Helper()
	b.cancel()
	return b.wait(t)
}

func want(t *testing.T, r result, code int, contains ...string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", r.code, code, r.out, r.errOut)
	}
	all := r.out + r.errOut
	for _, c := range contains {
		if !strings.Contains(all, c) {
			t.Fatalf("output lacks %q\nstdout: %s\nstderr: %s", c, r.out, r.errOut)
		}
	}
}

func jsonDoc(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, s)
	}
	if m["schemaVersion"] != "1.0" {
		t.Fatalf("schemaVersion %v\n%s", m["schemaVersion"], s)
	}
	return m
}

func jsonLines(t *testing.T, s string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l != "" {
			out = append(out, jsonDoc(t, l))
		}
	}
	return out
}

// resultOf returns the result object of a JSON envelope of command.
func resultOf(t *testing.T, r result, command string) map[string]any {
	t.Helper()
	m := jsonDoc(t, r.out)
	if m["command"] != command {
		t.Fatalf("command %v, want %q\n%s", m["command"], command, r.out)
	}
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result object\n%s", r.out)
	}
	return res
}

// errorOf returns the error object of a JSON error document.
func errorOf(t *testing.T, r result) map[string]any {
	t.Helper()
	m := jsonDoc(t, r.out)
	e, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error document\n%s", r.out)
	}
	return e
}

// testServer is a go-knx KNXnet/IP server on 127.0.0.1 with a virtual
// device:
//
//	1/2/3 switch  DPT 1.001 CRWU  off
//	1/2/4 temp    DPT 9.001 CRT   21.5
//	1/2/5 step    DPT 3.007 CWU   (6-bit value)
//	1/2/6 setp    DPT 9.001 CRWU  (no value)
type testServer struct {
	*server.Server
	addr string
	dev  *device.Device

	mu        sync.Mutex
	telegrams []server.Telegram
}

var tunnelAddrs = []knx.IndividualAddress{
	knx.MustParseIndividualAddress("1.1.10"), knx.MustParseIndividualAddress("1.1.11"),
	knx.MustParseIndividualAddress("1.1.12"), knx.MustParseIndividualAddress("1.1.13"),
}

func startServer(t *testing.T, opts ...server.Option) *testServer {
	t.Helper()
	opts = append([]server.Option{server.WithListenAddr("127.0.0.1:0"), server.WithDiscovery(false),
		server.WithName("knxctl test"), server.WithTunnelAddresses(tunnelAddrs...)}, opts...)
	s, err := server.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	d := device.New("test device", knx.MustParseIndividualAddress("1.1.1"))
	for _, o := range []struct{ name, dpt, flags, ga, value string }{
		{"switch", "1.001", "CRWU", "1/2/3", "off"},
		{"temp", "9.001", "CRT", "1/2/4", "21.5"},
		{"step", "3.007", "CWU", "1/2/5", ""},
		{"setp", "9.001", "CRWU", "1/2/6", ""},
	} {
		obj, err := device.NewGroupObject(o.name, o.dpt, o.flags, o.ga)
		if err != nil {
			t.Fatal(err)
		}
		if o.value != "" {
			v, err := obj.Type.Parse(o.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := obj.Set(v); err != nil {
				t.Fatal(err)
			}
		}
		d.Add(obj)
	}
	s.AddDevice(d)
	ts := &testServer{Server: s, dev: d}
	s.Subscribe(func(tg server.Telegram) {
		ts.mu.Lock()
		ts.telegrams = append(ts.telegrams, tg)
		ts.mu.Unlock()
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ts.addr = s.Addr().String()
	return ts
}

// object returns the raw value of a group object of the device.
func (s *testServer) object(t *testing.T, name string) []byte {
	t.Helper()
	o, ok := s.dev.Object(name)
	if !ok {
		t.Fatalf("no object %s", name)
	}
	b, _ := o.Raw()
	return b
}

// last returns the last telegram to g of service svc.
func (s *testServer) last(g string, svc apdu.APCI) (apdu.GroupValue, server.Telegram, bool) {
	ga := knx.MustParseGroupAddress(g)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.telegrams) - 1; i >= 0; i-- {
		tg := s.telegrams[i]
		if dst, ok := tg.Frame.Group(); !ok || dst != ga {
			continue
		}
		got, v, err := apdu.ParseGroupValue(tg.Frame.TPDU)
		if err == nil && got == svc {
			return v, tg, true
		}
	}
	return apdu.GroupValue{}, server.Telegram{}, false
}

// eventually waits for cond.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// deadPort returns a UDP endpoint on 127.0.0.1 nobody listens on.
func deadPort(t *testing.T) string {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	addr := c.LocalAddr().String()
	_ = c.Close()
	return addr
}

// multicast probing for the routing tests.
var (
	mcOnce sync.Once
	mcErr  error
)

// loopbackInterface is the loopback interface when it supports multicast
// (macOS, BSD), or "" (Linux: the system's choice).
func loopbackInterface() string {
	ifs, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 && i.Flags&net.FlagMulticast != 0 && i.Flags&net.FlagUp != 0 {
			return i.Name
		}
	}
	return ""
}

// requireMulticast skips the test without IPv4 multicast loopback, or
// fails it with KNX_REQUIRE_MULTICAST=1.
func requireMulticast(t *testing.T) {
	t.Helper()
	mcOnce.Do(func() { mcErr = probeMulticast() })
	if mcErr == nil {
		return
	}
	if os.Getenv("KNX_REQUIRE_MULTICAST") == "1" {
		t.Fatalf("multicast is required (KNX_REQUIRE_MULTICAST=1) but does not work: %v", mcErr)
	}
	t.Skipf("multicast loopback does not work here (%v); KNX_REQUIRE_MULTICAST=1 makes this a failure", mcErr)
}

func probeMulticast() error {
	group := testGroup(nil)
	ropts := []routing.Option{routing.WithGroup(group.String()), routing.WithLoopback(true)}
	if name := loopbackInterface(); name != "" {
		ropts = append(ropts, routing.WithInterface(name))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rx, err := client.ListenRouting(ctx, ropts)
	if err != nil {
		return err
	}
	defer rx.Close()
	tx, err := client.ListenRouting(ctx, ropts, client.WithSource(knx.MustParseIndividualAddress("1.1.99")))
	if err != nil {
		return err
	}
	defer tx.Close()
	ga := knx.MustParseGroupAddress("31/7/255")
	sub := rx.Subscribe(4, ga)
	for i := 0; i < 5; i++ {
		if err := tx.GroupWrite(ctx, ga, true); err != nil {
			return err
		}
		select {
		case <-sub.C():
			return nil
		case <-time.After(300 * time.Millisecond):
		}
	}
	return fmt.Errorf("no loopback of a routing indication to %s", group)
}

// testGroup returns a fresh administratively scoped multicast group.
func testGroup(t *testing.T) netip.AddrPort {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		return netip.AddrPort{}
	}
	port := uint16(c.LocalAddr().(*net.UDPAddr).Port)
	_ = c.Close()
	return netip.AddrPortFrom(netip.AddrFrom4([4]byte{239, 255, byte(port >> 8), byte(port)}), port)
}
