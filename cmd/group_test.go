// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/cemi"
	"github.com/otfabric/knxctl/internal/errs"
)

func TestGroupWrite(t *testing.T) {
	s := startServer(t)
	// A boolean without --dpt is DPT 1.
	r := run(t, "group", "write", "1/2/3", "on", "-g", s.addr)
	want(t, r, 0, "write 1/2/3 01 sent via tunnel "+s.addr)
	if v := s.object(t, "switch"); !bytes.Equal(v, []byte{1}) {
		t.Fatalf("switch %X", v)
	}
	v, tg, ok := s.last("1/2/3", apdu.GroupValueWriteService)
	if !ok || !v.Small || tg.Frame.Source != tunnelAddrs[0] {
		t.Fatalf("telegram %v %+v", ok, v)
	}
	// --dpt, as JSON.
	res := resultOf(t, run(t, "group", "write", "1/2/6", "21.5", "--dpt", "9.001", "-g", s.addr, "--format", "json"), "group write")
	if res["data"] != "0C33" || res["dpt"] != "9.001" || res["small"] != false || res["service"] != "write" || res["group"] != "1/2/6" {
		t.Fatalf("write %v", res)
	}
	if v := s.object(t, "setp"); !bytes.Equal(v, []byte{0x0C, 0x33}) {
		t.Fatalf("setp %X", v)
	}
	// A negative value after --.
	want(t, run(t, "group", "write", "--dpt", "9.001", "-g", s.addr, "--", "1/2/6", "-30"), 0, "(DPT 9.001)")
	if v := s.object(t, "setp"); !bytes.Equal(v, []byte{0x8A, 0x24}) {
		t.Fatalf("setp %X", v)
	}
	// --raw octets.
	want(t, run(t, "group", "write", "1/2/6", "--raw", "0x0C 1A", "-g", s.addr), 0, "write 1/2/6 0C1A")
	if v := s.object(t, "setp"); !bytes.Equal(v, []byte{0x0C, 0x1A}) {
		t.Fatalf("setp %X", v)
	}
	// --raw --small: a 6-bit value in the APCI.
	want(t, run(t, "group", "write", "1/2/5", "--raw", "0D", "--small", "-g", s.addr), 0, "write 1/2/5 0D")
	if v := s.object(t, "step"); !bytes.Equal(v, []byte{0x0D}) {
		t.Fatalf("step %X", v)
	}
	if v, _, _ := s.last("1/2/5", apdu.GroupValueWriteService); !v.Small {
		t.Fatalf("not small: %+v", v)
	}
	// The gateway from the environment, and a requested tunnelling address.
	t.Setenv("KNXCTL_GATEWAY", s.addr)
	want(t, run(t, "group", "write", "1/2/3", "off", "--tunnel-address", "1.1.13", "--nat"), 0, "sent via tunnel")
	if v := s.object(t, "switch"); !bytes.Equal(v, []byte{0}) {
		t.Fatalf("switch %X", v)
	}
	if _, tg, _ := s.last("1/2/3", apdu.GroupValueWriteService); tg.Frame.Source != tunnelAddrs[3] {
		t.Fatalf("source %s", tg.Frame.Source)
	}
}

func TestGroupWriteInvalid(t *testing.T) {
	s := startServer(t)
	cases := []struct {
		args []string
		code string
	}{
		{[]string{"group", "write", "32/0/0", "on"}, "invalid_address"},
		{[]string{"group", "write", "1/2/3", "maybe"}, "invalid_value"},
		{[]string{"group", "write", "1/2/3", "on", "--dpt", "nope"}, "invalid_input"},
		{[]string{"group", "write", "1/2/3", "on", "--raw", "01"}, "invalid_input"},
		{[]string{"group", "write", "1/2/3", "--raw", "zz"}, "invalid_value"},
		{[]string{"group", "write", "1/2/3", "--raw", "40", "--small"}, "invalid_value"},
		{[]string{"group", "write", "1/2/3", "--raw", "0102", "--small"}, "invalid_value"},
		{[]string{"group", "write", "1/2/6", "1e9", "--dpt", "9.001"}, "invalid_value"},
		{[]string{"group", "write", "1/2/3", "on", "--routing"}, "invalid_input"},
		{[]string{"group", "write", "1/2/3", "on", "--source", "x"}, "invalid_input"},
		{[]string{"group", "write", "1/2/3", "on", "--tunnel-address", "x"}, "invalid_input"},
		{[]string{"group", "respond", "1/2/3", "maybe"}, "invalid_value"},
		{[]string{"group", "read", "1/2/x"}, "invalid_address"},
		{[]string{"group", "read", "1/2/3", "--dpt", "nope"}, "invalid_input"},
		{[]string{"group", "listen", "1/2/x"}, "invalid_address"},
		{[]string{"group", "listen", "--map", "1/2/3"}, "invalid_input"},
		{[]string{"group", "listen", "--map", "x=1.001"}, "invalid_input"},
		{[]string{"group", "listen", "--map", "1/2/3=nope"}, "invalid_input"},
		{[]string{"group", "listen", "--dpt", "nope"}, "invalid_input"},
		{[]string{"monitor", "--dpt", "nope"}, "invalid_input"},
		{[]string{"group", "answer", "1/2/x", "on"}, "invalid_address"},
		{[]string{"group", "answer", "1/2/3", "on", "--dpt", "nope"}, "invalid_input"},
		{[]string{"group", "answer", "1/2/3", "maybe"}, "invalid_value"},
	}
	for _, c := range cases {
		args := append(c.args, "-g", s.addr, "--format", "json")
		r := run(t, args...)
		want(t, r, errs.ExitInvalidInput)
		if e := errorOf(t, r); e["kind"] != "invalid_input" || e["code"] != c.code {
			t.Fatalf("%v: error %v", c.args, e)
		}
	}
	// Nothing was sent.
	s.mu.Lock()
	n := len(s.telegrams)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d telegrams sent by invalid commands", n)
	}
	// A gateway nobody answers.
	r := run(t, "group", "write", "1/2/3", "on", "-g", deadPort(t), "--timeout", "300ms", "--format", "json")
	if r.code != errs.ExitTimeout && r.code != errs.ExitTransport {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	errorOf(t, r)
}

func TestGroupRespond(t *testing.T) {
	s := startServer(t)
	want(t, run(t, "group", "respond", "1/2/3", "on", "-g", s.addr), 0, "response 1/2/3 01 sent via tunnel")
	if _, _, ok := s.last("1/2/3", apdu.GroupValueResponseService); !ok {
		t.Fatal("no response telegram")
	}
	// The U flag takes the response over.
	if v := s.object(t, "switch"); !bytes.Equal(v, []byte{1}) {
		t.Fatalf("switch %X", v)
	}
	res := resultOf(t, run(t, "group", "respond", "1/2/6", "--dpt", "9.001", "-g", s.addr, "--format", "json", "--", "-5"), "group respond")
	if res["service"] != "response" || res["dpt"] != "9.001" {
		t.Fatalf("respond %v", res)
	}
}

func TestGroupRead(t *testing.T) {
	s := startServer(t)
	r := run(t, "group", "read", "1/2/4", "--dpt", "9.001", "-g", s.addr)
	want(t, r, 0, "1.1.1", "-> 1/2/4", "response", "0C33 (21.5 °C)")
	res := resultOf(t, run(t, "group", "read", "1/2/4", "-t", "9.001", "-g", s.addr, "--format", "json"), "group read")
	rs := res["responses"].([]any)
	if len(rs) != 1 {
		t.Fatalf("responses %v", rs)
	}
	resp := rs[0].(map[string]any)
	if resp["source"] != "1.1.1" || resp["service"] != "response" || resp["data"] != "0C33" || resp["value"] != 21.5 || resp["text"] != "21.5 °C" || resp["dpt"] != "9.001" {
		t.Fatalf("response %v", resp)
	}
	// Without --dpt: the octets only. A small value.
	res = resultOf(t, run(t, "group", "read", "1/2/3", "-g", s.addr, "--format", "json"), "group read")
	resp = res["responses"].([]any)[0].(map[string]any)
	if resp["data"] != "00" || resp["small"] != true || resp["value"] != nil {
		t.Fatalf("response %v", resp)
	}
	// A datapoint type that does not fit the value.
	res = resultOf(t, run(t, "group", "read", "1/2/4", "--dpt", "1.001", "-g", s.addr, "--format", "json"), "group read")
	if resp = res["responses"].([]any)[0].(map[string]any); resp["decodeError"] == nil {
		t.Fatalf("response %v", resp)
	}
	want(t, run(t, "group", "read", "1/2/4", "--dpt", "1.001", "-g", s.addr), 0, "undecodable")
	// --all collects every response in the window.
	res = resultOf(t, run(t, "group", "read", "1/2/4", "--all", "300ms", "-g", s.addr, "--format", "json"), "group read")
	if rs := res["responses"].([]any); len(rs) != 1 {
		t.Fatalf("responses %v", rs)
	}
	want(t, run(t, "group", "read", "1/2/4", "--all", "200ms", "--dpt", "9.001", "-g", s.addr), 0, "(21.5 °C)")
}

func TestGroupReadTimeout(t *testing.T) {
	s := startServer(t)
	// Nobody has the R flag on 1/2/5 (and nobody has 7/7/7).
	for _, g := range []string{"1/2/5", "7/7/7"} {
		r := run(t, "group", "read", g, "-g", s.addr, "--timeout", "300ms", "--format", "json")
		want(t, r, errs.ExitTimeout)
		e := errorOf(t, r)
		if e["kind"] != "timeout" || e["code"] != "timeout" || !strings.Contains(e["message"].(string), g) {
			t.Fatalf("error %v", e)
		}
	}
	want(t, run(t, "group", "read", "7/7/7", "-g", s.addr, "--timeout", "300ms"), errs.ExitTimeout, "error:", "no response")
	// --all without a response: exit 5, one error document.
	r := run(t, "group", "read", "7/7/7", "--all", "200ms", "-g", s.addr, "--timeout", "300ms", "--format", "json")
	want(t, r, errs.ExitTimeout)
	if e := errorOf(t, r); e["code"] != "no_response" || e["kind"] != "timeout" {
		t.Fatalf("error %v", e)
	}
	r = run(t, "group", "read", "7/7/7", "--all", "200ms", "-g", s.addr, "--timeout", "300ms")
	want(t, r, errs.ExitTimeout, "no response to the read of 7/7/7")
}

func TestGroupAnswer(t *testing.T) {
	s := startServer(t)
	b := start(t, "group", "answer", "5/5/5", "42", "--dpt", "5.010", "--count", "2", "-g", s.addr)
	b.waitErr(t, "answering reads of 5/5/5")
	for i := 0; i < 2; i++ {
		r := run(t, "group", "read", "5/5/5", "--dpt", "5.010", "-g", s.addr, "--format", "json")
		res := resultOf(t, r, "group read")
		resp := res["responses"].([]any)[0].(map[string]any)
		if resp["value"] != 42.0 || resp["source"] != tunnelAddrs[0].String() {
			t.Fatalf("response %v", resp)
		}
	}
	want(t, b.wait(t), 0, "answered 2 reads of 5/5/5")

	// Writes to the group are not answered; --duration ends it; --raw.
	b = start(t, "group", "answer", "5/5/6", "--raw", "2A", "--duration", "1s", "-g", s.addr, "--format", "json")
	b.waitErr(t, "answering reads of 5/5/6")
	want(t, run(t, "group", "write", "5/5/6", "on", "-g", s.addr), 0)
	r := run(t, "group", "read", "5/5/6", "-g", s.addr, "--format", "json")
	if resp := resultOf(t, r, "group read")["responses"].([]any)[0].(map[string]any); resp["data"] != "2A" {
		t.Fatalf("response %v", resp)
	}
	r = b.wait(t)
	want(t, r, 0)
	if res := resultOf(t, r, "group answer"); res["answered"] != 1.0 || res["group"] != "5/5/6" {
		t.Fatalf("answer %v", res)
	}

	// The server going away ends it with an error.
	b = start(t, "group", "answer", "5/5/7", "on", "-g", s.addr)
	b.waitErr(t, "answering reads")
	_ = s.Close()
	r = b.wait(t)
	if r.code == 0 {
		t.Fatalf("exit 0 after the server closed\n%s%s", r.out, r.errOut)
	}
}

// traffic writes v to g as the server every few milliseconds until the
// command ends.
func traffic(t *testing.T, s *testServer, b *background, g string, data []byte, small bool) result {
	t.Helper()
	ga := knx.MustParseGroupAddress(g)
	for !b.ended() {
		_ = s.GroupWrite(ga, apdu.GroupValue{Data: data, Small: small})
		time.Sleep(5 * time.Millisecond)
	}
	return b.wait(t)
}

func TestGroupListen(t *testing.T) {
	s := startServer(t)
	b := start(t, "group", "listen", "1/2/4", "--map", "1/2/4=9.001", "--count", "3", "-g", s.addr, "--format", "json")
	b.waitErr(t, "listening via tunnel")
	// Telegrams to other groups are not shown.
	_ = s.GroupWrite(knx.MustParseGroupAddress("1/2/3"), apdu.GroupValue{Data: []byte{1}, Small: true})
	r := traffic(t, s, b, "1/2/4", []byte{0x0C, 0x33}, false)
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	if len(recs) != 3 {
		t.Fatalf("%d records\n%s", len(recs), r.out)
	}
	for _, rec := range recs {
		if rec["type"] != "telegram" || rec["group"] != "1/2/4" || rec["source"] != "15.15.0" || rec["service"] != "write" ||
			rec["data"] != "0C33" || rec["dpt"] != "9.001" || rec["value"] != 21.5 || rec["text"] != "21.5 °C" || rec["time"] == nil {
			t.Fatalf("record %v", rec)
		}
	}
	// Text lines; the read telegrams of the tunnel are seen too.
	b = start(t, "group", "listen", "--count", "1", "-g", s.addr)
	b.waitErr(t, "listening via tunnel")
	r = traffic(t, s, b, "1/2/3", []byte{1}, true)
	want(t, r, 0, "15.15.0", "-> 1/2/3", "write", " 01")
	if n := strings.Count(strings.TrimSpace(r.out), "\n"); n != 0 {
		t.Fatalf("%d lines\n%s", n+1, r.out)
	}
	// --duration ends a quiet listen with exit 0 and no output.
	r = run(t, "group", "listen", "9/7/9", "--duration", "200ms", "-g", s.addr)
	want(t, r, 0)
	if r.out != "" {
		t.Fatalf("stdout %q", r.out)
	}
	// -q: nothing on stderr.
	r = run(t, "group", "listen", "9/7/9", "--duration", "100ms", "-g", s.addr, "-q")
	want(t, r, 0)
	if r.errOut != "" {
		t.Fatalf("stderr %q", r.errOut)
	}
}

func TestMonitor(t *testing.T) {
	s := startServer(t)
	// --dpt decodes every group; a value that does not fit is reported.
	b := start(t, "monitor", "--dpt", "9.001", "--count", "2", "-g", s.addr, "--format", "json")
	b.waitErr(t, "listening via")
	r := traffic(t, s, b, "1/2/4", []byte{0x0C, 0x33}, false)
	want(t, r, 0)
	for _, rec := range jsonLines(t, r.out) {
		if rec["value"] != 21.5 || rec["dpt"] != "9.001" {
			t.Fatalf("record %v", rec)
		}
	}
	b = start(t, "monitor", "--dpt", "9.001", "-n", "1", "-g", s.addr)
	b.waitErr(t, "listening via")
	r = traffic(t, s, b, "1/2/3", []byte{1}, true)
	want(t, r, 0, "undecodable")
	// --map takes precedence over --dpt; reads have no data.
	b = start(t, "monitor", "--map", "1/2/3=1.001", "--dpt", "9.001", "-n", "2", "-g", s.addr, "--format", "json")
	b.waitErr(t, "listening via")
	_ = s.Send(readFrame(t, "1/2/3"))
	r = traffic(t, s, b, "1/2/3", []byte{1}, true)
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	if recs[0]["service"] != "read" || recs[0]["data"] != nil {
		t.Fatalf("read record %v", recs[0])
	}
	if recs[1]["dpt"] != "1.001" || recs[1]["value"] != true || recs[1]["text"] != "on" || recs[1]["small"] != true {
		t.Fatalf("write record %v", recs[1])
	}
	// Ctrl-C ends the monitor normally.
	b = start(t, "monitor", "-g", s.addr)
	b.waitErr(t, "listening via")
	want(t, b.stop(t), 0)
	// The server closing the connection ends it with an error.
	b = start(t, "monitor", "-g", s.addr, "--format", "json")
	b.waitErr(t, "listening via")
	_ = s.Close()
	r = b.wait(t)
	if r.code != errs.ExitTransport {
		t.Fatalf("exit %d\n%s%s", r.code, r.out, r.errOut)
	}
	if e := errorOf(t, r); e["kind"] != "transport" {
		t.Fatalf("error %v", e)
	}
}

// readFrame is A_GroupValue_Read of g from the server.
func readFrame(t *testing.T, g string) *cemi.LData {
	t.Helper()
	return cemi.NewGroupFrame(cemi.LDataInd, 0, knx.MustParseGroupAddress(g), knx.PriorityLow, apdu.GroupValueRead())
}
