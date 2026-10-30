// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/datasecure"
	"github.com/otfabric/go-knx/keyring"
	"github.com/otfabric/go-knx/server"
)

const toolKeyHex = "746f6f6c2d6b65792d666f722d746573"

var hostIA = knx.MustParseIndividualAddress("1.1.0")

// toolKeyring is testKeyring with a tool key for the host device 1.1.0.
func toolKeyring(t *testing.T) string {
	t.Helper()
	kr, err := keyring.LoadFile(testKeyring(t), "kpw")
	if err != nil {
		t.Fatal(err)
	}
	copy(kr.Devices[0].ToolKey[:], "tool-key-for-tes")
	kr.Devices[0].HasToolKey = true
	b, err := kr.Encode("kpw")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "tool.knxkeys")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRemoteProperty(t *testing.T) {
	s := startServer(t, server.WithIndividualAddress(hostIA))
	want(t, run(t, "property", "read", s.addr, "knxnetip", "76", "--count", "11", "--device", "1.1.0"), 0, `1.1.0 object 0x000B (index 2) property 76 [1..11]`, `"knxctl test"`)
	want(t, run(t, "property", "read", s.addr, "device", "11", "--device", "1.1.0", "--connected", "--format", "json"), 0, `"objectIndex": 0`, `"device": "1.1.0"`)
	want(t, run(t, "property", "write", s.addr, "knxnetip", "76", "6B6E78", "--count", "3", "--device", "1.1.0", "--connected"), 0, `"knx"`)
	want(t, run(t, "property", "write", s.addr, "device", "11", "000000000000", "--device", "1.1.0"), 9)
	want(t, run(t, "property", "read", s.addr, "0x0099", "1", "--device", "1.1.0"), 9, "no interface object")
	for _, args := range [][]string{
		{"read", s.addr, "device", "11", "--connected"},
		{"read", s.addr, "device", "11", "--tool-key", toolKeyHex},
		{"read", s.addr, "device", "11", "--device", "x"},
		{"read", s.addr, "device", "11", "--device", "1.1.0", "--tool-key", "zz"},
		{"read", s.addr, "device", "11", "--device", "1.1.0", "--instance", "2"},
		{"write", s.addr, "device", "11", "01", "--connected"},
		{"write", s.addr, "device", "11", "01", "--device", "1.1.0", "--instance", "2"},
	} {
		want(t, run(t, append([]string{"property"}, args...)...), 3)
	}

	// A device with a tool key: secured access, plain access unanswered.
	var key datasecure.Key
	copy(key[:], "tool-key-for-tes")
	sec := startServer(t, server.WithIndividualAddress(hostIA), server.WithToolKey(key))
	want(t, run(t, "property", "read", sec.addr, "device", "11", "--device", "1.1.0", "--tool-key", toolKeyHex), 0, "[tool access]")
	want(t, run(t, "property", "read", sec.addr, "device", "11", "--device", "1.1.0", "--timeout", "1s"), 5)
}

func TestServeToolKey(t *testing.T) {
	kr := toolKeyring(t)
	b := start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--keyring", kr, "--keyring-password", "kpw", "--tool-key", "keyring")
	b.waitErr(t, "serving on")
	addr := regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	// The keyring gives the secure tunnel (user 2) and the device's tool key.
	want(t, run(t, "property", "read", addr, "knxnetip", "76", "--count", "4", "--device", "1.1.0", "--keyring", kr, "--keyring-password", "kpw", "--user-id", "2", "--format", "json"), 0, `"secured": true`)
	b.stop(t)

	b = start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--address", "1.1.0", "--tool-key", toolKeyHex)
	b.waitErr(t, "serving on")
	addr = regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	want(t, run(t, "property", "read", addr, "device", "11", "--device", "1.1.0", "--tool-key", toolKeyHex), 0, "[tool access]")
	b.stop(t)

	for _, args := range [][]string{
		{"--tool-key", "zz"},
		{"--tool-key", "keyring"},
		{"--tool-key", "keyring", "--keyring", testKeyring(t), "--keyring-password", "kpw"},
		{"--tool-key", "keyring", "--keyring", kr, "--keyring-password", "kpw", "--address", "x"},
	} {
		r := run(t, append([]string{"serve", "--listen", "127.0.0.1:0", "--no-discovery"}, args...)...)
		if r.code != 3 {
			t.Errorf("%v: exit %d %s", args, r.code, r.errOut)
		}
	}
}
