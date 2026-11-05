// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
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
	want(t, run(t, "property", "write", s.addr, "device", "11", "000000000000", "--device", "1.1.0"), 6)
	want(t, run(t, "property", "read", s.addr, "0x0099", "1", "--device", "1.1.0"), 6, "no interface object")
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

// A device with more than 256 interface objects: an object past index 255
// cannot be addressed and must not be found at a wrapped index.
func TestFindObjectBounds(t *testing.T) {
	list := make([]uint16, 300)
	for i := range list {
		list[i] = 0x1000 + uint16(i)
	}
	reads := 0
	read := func(start uint16, count uint8) ([]byte, error) {
		reads++
		var b []byte
		// A hostile device answers one entry more than asked.
		for i := int(start) - 1; i < int(start)-1+int(count)+1 && i < len(list); i++ {
			b = append(b, byte(list[i]>>8), byte(list[i]))
		}
		return b, nil
	}
	if idx, ok, err := findObject(len(list), 0x1000+255, read); err != nil || !ok || idx != 255 {
		t.Fatalf("index 255: %d %v %v", idx, ok, err)
	}
	// Index 256 would wrap to 0.
	if idx, ok, err := findObject(len(list), 0x1000+256, read); err != nil || ok {
		t.Fatalf("index 256 found at %d (%v)", idx, err)
	}
	// The extra entry of each answer is ignored: 0x1000+15 is only found
	// at its own index, from the second read.
	if idx, ok, _ := findObject(len(list), 0x1000+15, read); !ok || idx != 15 {
		t.Fatalf("index 15: %d %v", idx, ok)
	}
	reads = 0
	_, _, _ = findObject(65535, 0xFFFF, read)
	if reads > 18 {
		t.Fatalf("%d reads for a 65535-entry list", reads)
	}
}

// A read error ends the search with it; a device that answers fewer
// entries than asked has none beyond them.
func TestFindObjectErrors(t *testing.T) {
	boom := errors.New("boom")
	if _, _, err := findObject(20, 1, func(uint16, uint8) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("read error: %v", err)
	}
	short := func(start uint16, _ uint8) ([]byte, error) {
		if start == 1 {
			return []byte{0x10, 0x00, 0x10, 0x01}, nil // 2 of 15 entries
		}
		return []byte{0x10, 0x02}, nil
	}
	if idx, ok, err := findObject(20, 0x1001, short); err != nil || !ok || idx != 1 {
		t.Fatalf("entry in a short answer: %d %v %v", idx, ok, err)
	}
	if _, ok, err := findObject(20, 0x1002, short); err != nil || ok {
		t.Fatalf("entry past a short answer found (%v)", err)
	}
}
