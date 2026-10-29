// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/server"
)

func TestGroupOverTCP(t *testing.T) {
	s := startServer(t)
	r := run(t, "group", "write", "1/2/6", "19.5", "--dpt", "9.001", "--gateway", s.addr, "--tcp")
	want(t, r, 0, "write 1/2/6 079E")
	eventually(t, "the write", func() bool {
		v, _, ok := s.last("1/2/6", apdu.GroupValueWriteService)
		return ok && string(v.Data) == "\x07\x9e"
	})
	r = run(t, "group", "read", "1/2/4", "--dpt", "9.001", "--gateway", s.addr, "--tcp", "--format", "json")
	want(t, r, 0, `"value": 21.5`)
	r = run(t, "group", "read", "1/2/4", "--gateway", s.addr, "--tcp", "--nat")
	want(t, r, 3, "--tcp and --nat")
}

func TestBusmonitorCommand(t *testing.T) {
	s := startServer(t, server.WithConcurrentBusmonitor(true))
	b := start(t, "busmonitor", s.addr, "--count", "1", "--format", "json")
	b.waitErr(t, "busmonitor on")
	want(t, run(t, "group", "write", "1/2/3", "on", "--gateway", s.addr), 0)
	r := b.wait(t)
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	if len(recs) != 1 || recs[0]["type"] != "busmonitor" || recs[0]["destination"] != "1/2/3" ||
		recs[0]["service"] != "A_GroupValue_Write" || recs[0]["data"] != "01" || !strings.HasPrefix(recs[0]["raw"].(string), "BC") {
		t.Fatalf("records %v", recs)
	}

	// Text output, over TCP.
	b = start(t, "busmonitor", s.addr, "--count", "1", "--tcp")
	b.waitErr(t, "busmonitor on")
	want(t, run(t, "group", "write", "1/2/3", "off", "--gateway", s.addr), 0)
	want(t, b.wait(t), 0, "-> 1/2/3", "A_GroupValue_Write 00")

	// Without concurrent busmonitoring, a tunnel blocks it.
	ex := startServer(t)
	hold := start(t, "tunnel", "connect", ex.addr, "--hold", "5s")
	eventually(t, "the tunnel", func() bool { return len(ex.Connections()) == 1 })
	want(t, run(t, "busmonitor", ex.addr, "--count", "1"), 9, "E_NO_MORE_CONNECTIONS")
	hold.stop(t)
}

func TestPropertyRead(t *testing.T) {
	s := startServer(t)
	r := run(t, "property", "read", s.addr, "knxnetip", "76", "--count", "11")
	want(t, r, 0, `"knxctl test"`)
	r = run(t, "property", "read", s.addr, "0x000B", "53", "--start", "0", "--tcp", "--format", "json")
	want(t, r, 0, `"data": "0004"`)
	r = run(t, "property", "read", s.addr, "device", "99")
	want(t, r, 9, "void DP")
	want(t, run(t, "property", "read", s.addr, "nosuch", "1"), 3, "object")
	want(t, run(t, "property", "read", s.addr, "device", "300"), 3, "property id")
	want(t, run(t, "property", "read", s.addr, "device", "11", "--count", "0"), 3, "--count")
}
