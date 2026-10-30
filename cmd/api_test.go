// SPDX-License-Identifier: MIT

package cmd

import (
	"regexp"
	"testing"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/server"
)

func TestDiscoverFilters(t *testing.T) {
	plain := startServer(t)
	prog := startServer(t, server.WithProgrammingMode(true), server.WithName("in programming mode"))
	want(t, run(t, "discover", "--unicast", prog.addr, "--programming-mode"), 0, "in programming mode")
	r := run(t, "discover", "--unicast", plain.addr, "--programming-mode", "--timeout", "1s", "--format", "json")
	want(t, r, 0, `"devices": []`)
	want(t, run(t, "discover", "--unicast", plain.addr, "--service", "tunnelling:2"), 0, "knxctl test")
	r = run(t, "discover", "--unicast", plain.addr, "--service", "tunnelling:9", "--timeout", "1s", "--format", "json")
	want(t, r, 0, `"devices": []`)
	want(t, run(t, "discover", "--unicast", plain.addr, "--mac", "00:00:00:00:00:00", "--timeout", "1s"), 0)
	for _, args := range [][]string{{"--mac", "zz"}, {"--service", "nope"}, {"--service", "routing:0"}} {
		want(t, run(t, append([]string{"discover", "--unicast", plain.addr}, args...)...), 3)
	}
}

func TestPropertyWrite(t *testing.T) {
	s := startServer(t)
	// go-knx's server properties are read only: the write is refused.
	r := run(t, "property", "write", s.addr, "device", "54", "01")
	if r.code == 0 {
		t.Fatalf("write of a read-only property succeeded: %s", r.out)
	}
	for _, args := range [][]string{
		{"device", "54", "zz"},
		{"device", "300", "01"},
		{"nope", "54", "01"},
		{"device", "54", "0102", "--count", "3"},
		{"device", "54", "01", "--start", "0"},
	} {
		want(t, run(t, append([]string{"property", "write", s.addr}, args...)...), 3)
	}
}

func TestTunnelFeature(t *testing.T) {
	s := startServer(t)
	want(t, run(t, "tunnel", "feature", "get", s.addr, "max-apdu"), 0, "max-apdu (0x07): 00")
	want(t, run(t, "tunnel", "feature", "get", s.addr, "address", "--format", "json"), 0, `"feature": "address"`)
	want(t, run(t, "tunnel", "feature", "get", s.addr, "1", "--udp"), 0, "supported-emi (0x01): 0004")
	want(t, run(t, "tunnel", "feature", "set", s.addr, "info-service", "01"), 0, "set to 01")
	want(t, run(t, "tunnel", "feature", "set", s.addr, "max-apdu", "0010"), 9)
	want(t, run(t, "tunnel", "feature", "get", s.addr, "nope"), 3, "supported-emi")
	want(t, run(t, "tunnel", "feature", "set", s.addr, "info-service", "zz"), 3)
	// Secure.
	sec := startServer(t, secureOption(t))
	want(t, run(t, "tunnel", "feature", "get", sec.addr, "bus-connection", "--user-id", "2", "--user-password", "pw2"), 0, "01")
}

func TestDiagnoseSecure(t *testing.T) {
	s := startServer(t, secureOption(t))
	r := run(t, "diagnose", s.addr, "--format", "json")
	want(t, r, 1, "tunnelling needs a secure session")
	want(t, run(t, "diagnose", s.addr, "--user-id", "2", "--user-password", "pw2", "--device-password", "dev"), 0, "secured services")
	plain := startServer(t)
	want(t, run(t, "diagnose", plain.addr, "--tcp"), 0, "disconnect")
}

func TestPriorityAndServeOptions(t *testing.T) {
	s := startServer(t)
	want(t, run(t, "group", "write", "1/2/3", "on", "--gateway", s.addr, "--priority", "urgent"), 0)
	eventually(t, "the urgent write", func() bool {
		_, tg, ok := s.last("1/2/3", apdu.GroupValueWriteService)
		return ok && tg.Frame.Priority() == knx.PriorityUrgent
	})
	want(t, run(t, "group", "write", "1/2/3", "on", "--gateway", s.addr, "--priority", "nope"), 3, "--priority")

	b := start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--programming-mode", "--serial", "00FA12345678", "--name", "prog")
	b.waitErr(t, "serving on")
	addr := regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	want(t, run(t, "describe", addr, "--format", "json"), 0, `"serial": "00FA:12345678"`, `"programmingMode": true`)
	b.stop(t)
	want(t, run(t, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--serial", "123"), 3, "--serial")
}

func TestRoutingMonitorSecure(t *testing.T) {
	link := routingArgs(t)
	key := []string{"--backbone-key", "000102030405060708090a0b0c0d0e0f"}
	b := start(t, append(append([]string{"routing", "monitor", "--count", "1", "--map", "1/2/3=1.001"}, link...), key...)...)
	b.waitErr(t, "listening")
	want(t, routedWrites(t, b, append(link, key...), "1/2/3", "on"), 0, "1/2/3")
}
