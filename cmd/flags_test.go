// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/dpt"
	"github.com/otfabric/knxctl/internal/errs"
)

func TestFlagBit(t *testing.T) {
	for _, c := range []struct {
		dpt, in string
		bit     int
		err     string
	}{
		{"21.100", "forceRequest", 0, ""},
		{"21.100", "Force Request", 0, ""},
		{"21.100", "ROOM-H-MAX", 7, ""},
		{"21.100", "bit3", 3, ""},
		{"21.100", "BIT 7", 7, ""},
		// A part of exactly one name.
		{"21.001", "out of service", 0, ""},
		{"21.001", "in alarm", 3, ""},
		{"22.1000", "knx ip", 5, ""},
		{"26.001", "inactive", 6, ""},
		// A whole name wins over a part of another (overrun / oversupply).
		{"21.100", "overrun", 3, ""},
		{"21.100", "over", 0, "names more than one bit of 21.100 (bit2, bit3)"},
		{"21.001", "datapoint", 0, "names more than one bit"},
		{"21.100", "bit8", 0, "21.100 has bits bit0..bit7"},
		{"21.100", "bit01", 0, "is not a bit of 21.100"},
		{"21.100", "bogus", 0, `"bogus" is not a bit of 21.100 ("forceRequest", "protection"`},
		{"21.100", " - ", 0, "an empty bit name"},
	} {
		bit, err := flagBit(dpt.MustLookup(c.dpt), c.in)
		switch {
		case c.err == "" && (err != nil || bit != c.bit):
			t.Errorf("%s %q: bit %d, %v; want bit %d", c.dpt, c.in, bit, err, c.bit)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s %q: %v; want an error with %q", c.dpt, c.in, err, c.err)
		}
	}
}

func TestParseDPTValue(t *testing.T) {
	ty := dpt.MustLookup("21.100")
	for in, want := range map[string]uint64{
		"3":                       3,
		"0b101":                   5,
		"forceRequest,protection": 3,
		"protection, bit7":        0x82,
		"none":                    0,
		" None ":                  0,
	} {
		v, err := parseDPTValue(ty, in)
		if err != nil || v != want {
			t.Errorf("%q: %v, %v; want %d", in, v, err, want)
		}
	}
	// The number error comes first, then the names.
	_, err := parseDPTValue(ty, "protection,,bit1")
	if err == nil || !strings.Contains(err.Error(), "8-bit set") || !strings.Contains(err.Error(), "or bit names: an empty bit name") {
		t.Fatalf("error %v", err)
	}
	// Types without named bits parse as before.
	for _, d := range []string{"9.001", "21.1200"} {
		if _, err := parseDPTValue(dpt.MustLookup(d), "protection"); err == nil || strings.Contains(err.Error(), "bit names") {
			t.Errorf("%s: %v", d, err)
		}
	}
}

func TestSetFlags(t *testing.T) {
	ty := dpt.MustLookup("26.001")
	// The scene number (bits 0..5) and the reserved bit 7 are not named.
	if got := setFlags(ty, uint64(0xFF)); len(got) != 1 || got[0] != "scene is inactive" {
		t.Fatalf("26.001 FF: %q", got)
	}
	if got := setFlags(ty, uint64(0x3F)); got != nil {
		t.Fatalf("26.001 3F: %q", got)
	}
	if setFlags(dpt.MustLookup("9.001"), 21.5) != nil || setFlags(dpt.MustLookup("21.100"), 21.5) != nil {
		t.Fatal("names for a value that is not a bit set")
	}
	if withFlags("x", nil) != "x" || withFlags("x", []string{"a", "b"}) != "x: a, b" {
		t.Fatal("withFlags")
	}
	if typeFlags(dpt.MustLookup("9.001")) != nil {
		t.Fatal("9.001 has named bits")
	}
}

func TestDPTBitNames(t *testing.T) {
	// describe lists the named bits.
	want(t, run(t, "dpt", "describe", "21.100"), 0, "bits (least significant 0", " 0  forceRequest", " 7  roomHMax")
	r := run(t, "dpt", "describe", "26.001")
	want(t, r, 0, " 6  scene is inactive")
	if strings.Contains(r.out, " 0  ") {
		t.Fatalf("26.001 names bit 0:\n%s", r.out)
	}
	res := resultOf(t, run(t, "dpt", "describe", "22.1000", "--format", "json"), "dpt describe")
	nb := res["namedBits"].([]any)
	if first := nb[0].(map[string]any); len(nb) != 4 || first["bit"] != 1.0 || first["name"] != "TP1" {
		t.Fatalf("namedBits %v", nb)
	}
	res = resultOf(t, run(t, "dpt", "describe", "9.001", "--format", "json"), "dpt describe")
	if _, ok := res["namedBits"]; ok {
		t.Fatalf("9.001 %v", res)
	}
	// dpt list carries them too.
	res = resultOf(t, run(t, "dpt", "list", "--main", "21", "--format", "json"), "dpt list")
	if first := res["types"].([]any)[0].(map[string]any); first["id"] != "21.001" || first["namedBits"] == nil {
		t.Fatalf("list %v", first)
	}

	// decode names the set bits.
	want(t, run(t, "dpt", "decode", "21.100", "83"), 0, "83  0b10000011: forceRequest, protection, roomHMax  (DPT 21.100)")
	want(t, run(t, "dpt", "decode", "21.100", "00"), 0, "00  0b00000000  (DPT 21.100)")
	res = resultOf(t, run(t, "dpt", "decode", "22.1000", "0022", "--format", "json"), "dpt decode")
	if bn := res["bitNames"].([]any); res["text"] != "0b0000000000100010" || res["value"] != 34.0 || len(bn) != 2 || bn[0] != "TP1" || bn[1] != "KNX IP" {
		t.Fatalf("decode %v", res)
	}
	res = resultOf(t, run(t, "dpt", "decode", "21.100", "00", "--format", "json"), "dpt decode")
	if _, ok := res["bitNames"]; ok {
		t.Fatalf("no bit set: %v", res)
	}

	// encode takes names, bitN and parts of a name.
	want(t, run(t, "dpt", "encode", "21.100", "forceRequest,protection"), 0, "03  0b00000011: forceRequest, protection")
	want(t, run(t, "dpt", "encode", "21.001", "out of service, in alarm"), 0, "09  0b00001001")
	want(t, run(t, "dpt", "encode", "26.001", "inactive"), 0, "40  0b01000000: scene is inactive")
	want(t, run(t, "dpt", "encode", "21.100", "0x81"), 0, "81  0b10000001: forceRequest, roomHMax")
	want(t, run(t, "dpt", "encode", "21.100", "bogus"), errs.ExitInvalidInput, `"bogus" is not a bit of 21.100`)
	want(t, run(t, "dpt", "encode", "21.001", "datapoint"), errs.ExitInvalidInput, "names more than one bit")
	e := errorOf(t, run(t, "dpt", "encode", "21.100", "bit9", "--format", "json"))
	if e["kind"] != "invalid_input" || !strings.Contains(e["message"].(string), "bit0..bit7") {
		t.Fatalf("error %v", e)
	}
}

func TestGroupBitNames(t *testing.T) {
	s := startServer(t)
	// group write takes bit names.
	res := resultOf(t, run(t, "group", "write", "1/2/9", "protection,roomHMax", "--dpt", "21.100", "-g", s.addr, "--format", "json"), "group write")
	if res["data"] != "82" || res["dpt"] != "21.100" {
		t.Fatalf("write %v", res)
	}
	if v, _, ok := s.last("1/2/9", apdu.GroupValueWriteService); !ok || len(v.Data) != 1 || v.Data[0] != 0x82 {
		t.Fatalf("telegram %v %+v", ok, v)
	}
	// An unknown name is rejected before connecting.
	want(t, run(t, "group", "write", "1/2/9", "bogus", "--dpt", "21.100", "-g", s.addr), errs.ExitInvalidInput, "is not a bit of 21.100")

	// listen names the set bits, in JSON and in text.
	b := start(t, "group", "listen", "1/2/4", "--map", "1/2/4=21.100", "--count", "1", "-g", s.addr, "--format", "json")
	b.waitErr(t, "listening via tunnel")
	r := traffic(t, s, b, "1/2/4", []byte{0x03}, false)
	want(t, r, 0)
	rec := jsonLines(t, r.out)[0]
	if bn, _ := rec["bitNames"].([]any); rec["text"] != "0b00000011" || len(bn) != 2 || bn[0] != "forceRequest" || bn[1] != "protection" {
		t.Fatalf("record %v", rec)
	}
	b = start(t, "group", "listen", "1/2/4", "--map", "1/2/4=21.100", "--count", "1", "-g", s.addr)
	b.waitErr(t, "listening via tunnel")
	want(t, traffic(t, s, b, "1/2/4", []byte{0x03}, false), 0, "03 (0b00000011: forceRequest, protection)")
}

func TestServeBitNames(t *testing.T) {
	b, addr := serve(t, "--object", "1/3/4:21.100:CRWT=forceRequest,protection", "--format", "json")
	want(t, run(t, "group", "read", "1/3/4", "--dpt", "21.100", "-g", addr), 0, "03 (0b00000011: forceRequest, protection)")
	want(t, run(t, "group", "write", "1/3/4", "roomHMax", "--dpt", "21.100", "-g", addr), 0)
	r := b.stop(t)
	want(t, r, 0)
	var saw bool
	for _, rec := range jsonLines(t, r.out) {
		if bn, _ := rec["bitNames"].([]any); rec["group"] == "1/3/4" && rec["service"] == "write" && len(bn) == 1 && bn[0] == "roomHMax" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("no write record with bitNames:\n%s", r.out)
	}
	// An unknown name in an object's value is a configuration error.
	want(t, run(t, "serve", "--object", "1/3/4:21.100=bogus", "--listen", "127.0.0.1:0"), errs.ExitInvalidInput, "is not a bit of 21.100")
}
