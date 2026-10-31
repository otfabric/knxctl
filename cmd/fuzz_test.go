// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

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
