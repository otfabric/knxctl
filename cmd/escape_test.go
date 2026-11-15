// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/binary"
	"encoding/hex"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/keyring"
	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/go-knx/server"
)

func TestTermSafe(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"knxctl test", "knxctl test"},
		{"K\u00fcche 1.OG", "K\u00fcche 1.OG"},
		{`say "hi"`, `say "hi"`},
		{`a\b`, `a\\b`},
		{"\x1b[31mRED\x1b[0m", `\x1b[31mRED\x1b[0m`},
		{"a\rb\nc\td", `a\rb\nc\td`},
		{"\x00\x7f", `\x00\x7f`},
		{"\u0085\u009b", `\u0085\u009b`},
		{"\u202aL\u202bR\u202cP\u202dO\u202e", `\u202aL\u202bR\u202cP\u202dO\u202e`},
		{"\u2066\u2067\u2068\u2069", `\u2066\u2067\u2068\u2069`},
		{"\u200e\u200f\ufeff\u00ad\u00a0", `\u200e\u200f\ufeff\u00ad\u00a0`},
		{"\xff\xfeok", `\xff\xfeok`},
		{"\ufffd", "\ufffd"},
	} {
		if got := termSafe(c.in); got != c.want {
			t.Errorf("termSafe(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// controlName is a friendly name with terminal controls: CSI colour
// sequences, a carriage return and a C1 control (the name is Latin-1 on
// the wire).
const controlName = "\x1b[31mRED\x1b[0m\rFAKE\u009b"

// noTerminalControls fails the test when out holds a control character
// other than a newline, a C1 control or a bidirectional control.
func noTerminalControls(t *testing.T, what, out string) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Errorf("%s: invalid UTF-8: %q", what, out)
	}
	for _, r := range out {
		if (r < 0x20 && r != '\n') || r == 0x7f || (r >= 0x80 && r <= 0x9f) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			t.Errorf("%s: %U on stdout: %q", what, r, out)
			return
		}
	}
}

// A server's friendly name reaches the terminal escaped in table output.
func TestDiscoverTableEscapesNames(t *testing.T) {
	s := startServer(t, server.WithName(controlName))
	for _, format := range []string{"table", "text"} {
		r := run(t, "discover", "--unicast", s.addr, "--format", format)
		want(t, r, 0, `\x1b[31mRED\x1b[0m\rFAKE\u009b`)
		noTerminalControls(t, "discover --format "+format, r.out)
	}
	r := run(t, "describe", s.addr)
	want(t, r, 0, `\x1b[31mRED`)
	noTerminalControls(t, "describe", r.out)
	// JSON carries the name as it is.
	want(t, run(t, "discover", "--unicast", s.addr, "--format", "json"), 0, `"name": "\u001b[31mRED\u001b[0m\rFAKE`)
}

// decode prints the device of a search response with the name escaped.
func TestDecodeEscapesNames(t *testing.T) {
	dev := knxnetip.DeviceInfo{Medium: knxnetip.MediumIP, Address: knx.MustParseIndividualAddress("1.1.0"), Name: controlName}
	binary.BigEndian.PutUint16(dev.Serial[:], 0x00FA)
	families := knxnetip.ServiceFamilies{Families: []knxnetip.ServiceFamily{{ID: knxnetip.FamilyCore, Version: 1}}}
	for _, m := range []knxnetip.Message{
		&knxnetip.SearchResponse{Control: knxnetip.NewHPAI(netip.MustParseAddrPort("192.0.2.1:3671")), DIBs: knxnetip.DIBs{dev, families}},
		&knxnetip.DescriptionResponse{DIBs: knxnetip.DIBs{dev, families}},
	} {
		b, err := knxnetip.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		r := run(t, "decode", hex.EncodeToString(b))
		want(t, r, 0, `\x1b[31mRED\x1b[0m\rFAKE\u009b`)
		noTerminalControls(t, "decode", r.out)
	}
}

// keyring show prints the strings of the file escaped.
func TestKeyringShowEscapesStrings(t *testing.T) {
	kr, err := keyring.LoadFile(testKeyring(t), "kpw")
	if err != nil {
		t.Fatal(err)
	}
	kr.CreatedBy = "ETS\rsecure group 1/1/1 key 00\u202e"
	kr.Created = "2026\u2066"
	b, err := kr.Encode("kpw")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "creator.knxkeys")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, "keyring", "show", p, "--keyring-password", "kpw")
	want(t, r, 0, `created 2026\u2066 by ETS\rsecure group 1/1/1 key 00\u202e`)
	noTerminalControls(t, "keyring show", r.out)
}
