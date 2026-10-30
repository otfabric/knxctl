// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-knx/server"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
)

func TestDescribe(t *testing.T) {
	s := startServer(t)
	r := run(t, "describe", s.addr)
	want(t, r, 0, `"knxctl test"`, "individual address 15.15.0", "services:", "Tunnelling v")
	res := resultOf(t, run(t, "describe", s.addr, "--format", "json", "--nat"), "describe")
	d := res["device"].(map[string]any)
	if d["name"] != "knxctl test" || d["individualAddress"] != "15.15.0" || d["endpoint"] != s.addr {
		t.Fatalf("device %v", d)
	}
	if sv := d["services"].(map[string]any); sv["Tunnelling"] == nil || sv["Core"] == nil {
		t.Fatalf("services %v", sv)
	}
	// Nobody at the address: a timeout.
	r = run(t, "describe", deadPort(t), "--timeout", "300ms", "--format", "json")
	if r.code != errs.ExitTimeout && r.code != errs.ExitTransport {
		t.Fatalf("exit %d\n%s%s", r.code, r.out, r.errOut)
	}
	errorOf(t, r)
	want(t, run(t, "describe", "not a host:x", "--timeout", "300ms"), errs.ExitInvalidInput)
}

func TestDiscoverUnicast(t *testing.T) {
	s := startServer(t)
	r := run(t, "discover", "--unicast", s.addr, "--timeout", "500ms")
	want(t, r, 0, `"knxctl test"`, "individual address 15.15.0")
	r = run(t, "discover", "--unicast", s.addr, "--timeout", "500ms", "--format", "table", "--local-addr", "127.0.0.1")
	want(t, r, 0, "ENDPOINT", "NAME", "knxctl test")
	res := resultOf(t, run(t, "discover", "--unicast", s.addr, "--timeout", "500ms", "--extended", "--format", "json"), "discover")
	devs := res["devices"].([]any)
	if len(devs) != 1 {
		t.Fatalf("devices %v", devs)
	}
	d := devs[0].(map[string]any)
	if d["extended"] != true || d["name"] != "knxctl test" {
		t.Fatalf("device %v", d)
	}
	if o, ok := d["otherBlocks"].([]any); !ok || len(o) != 1 || o[0] != "EXTENDED_DEVICE_INFO" {
		t.Fatalf("other blocks %v", d["otherBlocks"])
	}
	// No answer: an empty list, not an error.
	r = run(t, "discover", "--unicast", deadPort(t), "--timeout", "300ms")
	want(t, r, 0, "no KNXnet/IP server answered")
	res = resultOf(t, run(t, "discover", "--unicast", deadPort(t), "--timeout", "300ms", "--format", "json"), "discover")
	if l := res["devices"].([]any); len(l) != 0 {
		t.Fatalf("devices %v", l)
	}
	want(t, run(t, "discover", "--repeat", "9", "--unicast", s.addr), errs.ExitInvalidInput)
	want(t, run(t, "discover", "--interface", "no-such-if0", "--unicast", s.addr), errs.ExitInvalidInput)
}

func TestTunnelConnect(t *testing.T) {
	s := startServer(t)
	r := run(t, "tunnel", "connect", s.addr)
	want(t, r, 0, "connected to "+s.addr, "channel", "address 1.1.10", "disconnected")
	res := resultOf(t, run(t, "tunnel", "connect", s.addr, "--hold", "300ms", "--heartbeat", "100ms", "--format", "json"), "tunnel connect")
	if res["individualAddress"] != "1.1.10" || res["held"] != "300ms" || res["heartbeatsOk"] != true || res["server"] != s.addr {
		t.Fatalf("tunnel %v", res)
	}
	want(t, run(t, "tunnel", "connect", s.addr, "--hold", "100ms"), 0, "held 100ms, connection alive: true")
	// The connection was closed: the server has no connection left.
	eventually(t, "disconnection", func() bool { return len(s.Connections()) == 0 })
	// A requested tunnelling address (extended CRI) and NAT.
	res = resultOf(t, run(t, "tunnel", "connect", s.addr, "--tunnel-address", "1.1.12", "--nat", "--local-addr", "127.0.0.1", "--format", "json"), "tunnel connect")
	if res["individualAddress"] != "1.1.12" {
		t.Fatalf("tunnel %v", res)
	}
	want(t, run(t, "tunnel", "connect", s.addr, "--tunnel-address", "x"), errs.ExitInvalidInput, "--tunnel-address")
	// A tunnelling address the server does not have is refused.
	r = run(t, "tunnel", "connect", s.addr, "--tunnel-address", "9.9.9", "--format", "json")
	want(t, r, errs.ExitProtocol)
	if e := errorOf(t, r); e["kind"] != "rejected" {
		t.Fatalf("error %v", e)
	}
	// Nobody at the address.
	r = run(t, "tunnel", "connect", deadPort(t), "--timeout", "300ms", "--format", "json")
	if r.code != errs.ExitTimeout && r.code != errs.ExitTransport {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
}

func TestTunnelConnectInterrupted(t *testing.T) {
	s := startServer(t)
	b := start(t, "tunnel", "connect", s.addr, "--hold", "1m")
	// The client is connected when it says so; the server registers the
	// connection before its CONNECT_RESPONSE arrives.
	b.waitErr(t, "holding for")
	r := b.stop(t)
	want(t, r, 0, "held 1m0s")
}

func TestTunnelConnectServerGone(t *testing.T) {
	s := startServer(t)
	b := start(t, "tunnel", "connect", s.addr, "--hold", "1m", "--format", "json")
	b.waitErr(t, "holding for")
	_ = s.Close() // disconnects the client
	r := b.wait(t)
	want(t, r, 0)
	if res := resultOf(t, r, "tunnel connect"); res["heartbeatsOk"] != false {
		t.Fatalf("tunnel %v", res)
	}
}

func TestDiagnose(t *testing.T) {
	s := startServer(t)
	r := run(t, "diagnose", s.addr)
	want(t, r, 0, "diagnose "+s.addr, "[ok  ] directed search", "[ok  ] heartbeat", "all checks passed")
	res := resultOf(t, run(t, "diagnose", s.addr, "--format", "json"), "diagnose")
	if res["ok"] != true || len(res["checks"].([]any)) != 6 {
		t.Fatalf("diagnose %v", res)
	}
	want(t, run(t, "diagnose", s.addr, "--format", "table"), 0, "CHECK", "RESULT", "tunnel connect", "ok")
}

func TestDiagnoseFailing(t *testing.T) {
	// Nobody answers: the description fails and the checks stop there.
	r := run(t, "diagnose", deadPort(t), "--timeout", "300ms")
	want(t, r, errs.ExitFindings, "[FAIL] directed search", "[FAIL] description", "some checks failed")
	if strings.Contains(r.out, "tunnel connect") {
		t.Fatalf("checks after a failed description:\n%s", r.out)
	}
	// A server whose only tunnelling slot is in use: the description
	// passes, the connection is refused.
	s := startServer(t, server.WithTunnelAddresses(tunnelAddrs[0]))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := tunnel.Dial(ctx, s.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r = run(t, "diagnose", s.addr, "--timeout", "500ms", "--format", "json")
	want(t, r, errs.ExitFindings)
	res := resultOf(t, r, "diagnose")
	if res["ok"] != false {
		t.Fatalf("diagnose %v", res)
	}
	var failed []string
	for _, c := range res["checks"].([]any) {
		c := c.(map[string]any)
		if c["ok"] != true {
			failed = append(failed, c["name"].(string))
		}
	}
	if strings.Join(failed, ",") != "tunnel connect" {
		t.Fatalf("failed checks %v\n%s", failed, r.out)
	}
	r = run(t, "diagnose", s.addr, "--timeout", "500ms", "--format", "table")
	want(t, r, errs.ExitFindings, "FAIL")
}
