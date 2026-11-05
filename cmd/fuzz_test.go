// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/cemi"
	"github.com/otfabric/go-knx/knxnetip"
)

func seedHex(f *testing.F, frames ...string) {
	f.Helper()
	for _, s := range frames {
		b, err := hex.DecodeString(s)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
}

// FuzzDecodeFrame feeds `knxctl decode` octets. A frame that decodes must
// render in both output formats: JSON cannot be left to fail after the
// command has decided the input is fine.
func FuzzDecodeFrame(f *testing.F) {
	seedHex(f,
		"061004200015040100001100bce0110a0a03010081",
		"06100530001129001100bce0110a0a03010081",
		"0610020500140801c0a80001000e0401000a0000",
		"06100421000a04010000",
		"0610ffff00060000",
	)
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := decodeFrame(b)
		if err != nil {
			return
		}
		if d.Length != len(b) || d.Service == "" {
			t.Fatalf("frame %x: %+v", b, d)
		}
		if d.Text() == "" {
			t.Fatalf("frame %x: empty text", b)
		}
		if _, err := json.Marshal(d); err != nil {
			t.Fatalf("frame %x: %v", b, err)
		}
		// What the parser accepted is a message the service of which is named.
		if m, err := knxnetip.Parse(b); err != nil || m.Service().String() != d.Service {
			t.Fatalf("frame %x: parsed differently: %v", b, err)
		}
	})
}

// FuzzDecodeCEMI feeds `knxctl decode --cemi` octets.
func FuzzDecodeCEMI(f *testing.F) {
	seedHex(f, "2900bce0110a0a03010081", "1100bce0110a0a03010081", "fc000000000000", "")
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := decodeCEMI(b)
		if err != nil {
			return
		}
		if !strings.EqualFold(d.Raw, hex.EncodeToString(b)) {
			t.Fatalf("cEMI %x: raw %s", b, d.Raw)
		}
		if _, err := json.Marshal(d); err != nil {
			t.Fatalf("cEMI %x: %v", b, err)
		}
	})
}

// FuzzParseFlags feeds the flag values that have a syntax of their own.
func FuzzParseFlags(f *testing.F) {
	for _, s := range []string{"1/2/3=9.001", "1/2/3:9.001:CRWU=21.5", "1/2+1/3:1.001", "device", "0x0B", "=", ":", "1/2/3:"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if m, err := parseDPTMap([]string{s}); err == nil && len(m) != 1 {
			t.Fatalf("%q: %d entries", s, len(m))
		}
		if o, err := parseObject(s); err == nil && (o.DPT == "" && !strings.Contains(s, ":")) {
			t.Fatalf("%q: %+v", s, o)
		}
		if _, err := parseGroup(s); err == nil && s == "" {
			t.Fatal("an empty group address parsed")
		}
		_, _ = parseObjectType(s)
		_, _ = parseFeature(s)
	})
}

// FuzzBusmonitorRecord feeds L_Busmon.ind frames as a hostile server
// sends them: the record of any frame renders as a line and as JSON.
func FuzzBusmonitorRecord(f *testing.F) {
	seedHex(f, "bc110a0a03e1008133", "b0110a0a0360", "", "ff", "bc110a0a03ff0081")
	f.Fuzz(func(t *testing.T, b []byte) {
		raw := &cemi.Raw{MsgCode: cemi.LBusmonInd, Data: b}
		if len(b) > 2 {
			raw.AddInfo = []cemi.AdditionalInfo{{Type: cemi.AddInfoBusmonitorStatus, Data: b[:1]}, {Type: cemi.AddInfoTimestamp, Data: b[1:3]}}
		}
		r := newBusmonitorRecord(raw)
		if r.line() == "" {
			t.Fatal("empty line")
		}
		if _, err := json.Marshal(r); err != nil {
			t.Fatal(err)
		}
		if !strings.EqualFold(r.Raw, hex.EncodeToString(b)) {
			t.Fatalf("raw %s of %x", r.Raw, b)
		}
	})
}

// FuzzFindObject feeds the interface object list answers of a hostile
// device: a found index is within the one-octet range and is the
// position of the type in what was asked for.
func FuzzFindObject(f *testing.F) {
	f.Add(300, uint16(0x000B), []byte{0x00, 0x00, 0x00, 0x0B})
	f.Add(0, uint16(1), []byte{})
	f.Add(-5, uint16(1), []byte{0x00, 0x01})
	f.Add(65535, uint16(0xFFFF), []byte{0xFF, 0xFF, 0xFF})
	f.Fuzz(func(t *testing.T, n int, objType uint16, answer []byte) {
		reads := 0
		idx, ok, err := findObject(n, objType, func(start uint16, count uint8) ([]byte, error) {
			reads++
			if count < 1 || count > 15 || start < 1 {
				t.Fatalf("read of %d elements from %d", count, start)
			}
			return answer, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if reads > 18 {
			t.Fatalf("%d reads", reads)
		}
		if ok {
			pos := int(idx) % 15 * 2
			if int(idx) >= max(n, 0) || pos+1 >= len(answer) || uint16(answer[pos])<<8|uint16(answer[pos+1]) != objType {
				t.Fatalf("index %d of %d entries, answer %x", idx, n, answer)
			}
		}
	})
}

// FuzzRawValue feeds --raw and --small: an accepted value is a group
// value go-knx can encode.
func FuzzRawValue(f *testing.F) {
	for _, s := range []string{"0C33", "0x01", "0X3F", "40", "", "0x", "A5 A5", strings.Repeat("00", 255)} {
		f.Add(s, false)
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, raw string, small bool) {
		gv, err := parseValue(nil, raw, small, "")
		if err != nil {
			return
		}
		if len(gv.Data) == 0 || len(gv.Data) > maxGroupValue || small && (len(gv.Data) != 1 || gv.Data[0] > 0x3F) {
			t.Fatalf("%q small %v: %x", raw, small, gv.Data)
		}
		if _, err := apdu.GroupValueWrite(gv); err != nil {
			t.Fatalf("%q: accepted, but %v", raw, err)
		}
	})
}

// FuzzSearchFilters feeds --mac and --service.
func FuzzSearchFilters(f *testing.F) {
	for _, s := range []string{"aa:bb:cc:dd:ee:ff", "tunnelling:2", "core", "routing:0", "security:256", "aa-bb-cc-dd-ee-ff", ":"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, ps := range [][2]string{{s, ""}, {"", s}} {
			params, err := searchParams(false, ps[0], ps[1])
			if err != nil {
				continue
			}
			for _, p := range params {
				if p.Type == knxnetip.SRPMACAddress && len(p.Data) != 6 || p.Type == knxnetip.SRPService && (len(p.Data) != 2 || p.Data[1] == 0) {
					t.Fatalf("%q: %+v", s, p)
				}
			}
		}
	})
}

// FuzzRedact feeds --secure-user arguments: the password never shows.
func FuzzRedact(f *testing.F) {
	for _, s := range []string{"2:secret:1.1.10", "2:secret", "nocolon", "::", "2:pw:with:colons"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r := redact(s)
		parts := strings.SplitN(s, ":", 3)
		if len(parts) < 2 {
			if r != s {
				t.Fatalf("%q -> %q", s, r)
			}
			return
		}
		want := parts[0] + ":***"
		if len(parts) == 3 {
			want += ":" + parts[2]
		}
		if r != want {
			t.Fatalf("%q -> %q", s, r)
		}
	})
}
