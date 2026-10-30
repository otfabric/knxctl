// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/datasecure"
	"github.com/otfabric/go-knx/keyring"
	"github.com/otfabric/go-knx/secure"
	"github.com/otfabric/go-knx/server"
)

func secureOption(t *testing.T) server.Option {
	t.Helper()
	dev, _ := secure.DeviceAuthenticationCode("dev")
	u2, _ := secure.UserPasswordKey("pw2")
	u3, _ := secure.UserPasswordKey("pw3")
	u1, _ := secure.UserPasswordKey("pw1")
	return server.WithSecure(dev, map[uint8]server.SecureUser{1: {Key: u1}, 2: {Key: u2, Address: tunnelAddrs[0]}, 3: {Key: u3, Address: tunnelAddrs[1]}})
}

// testKeyring writes a keyring matching secureOption: users 2 and 3 of
// host 1.1.0 and the Data Secure group 1/2/9.
func testKeyring(t *testing.T) string {
	t.Helper()
	g := knx.MustParseGroupAddress("1/2/9")
	kr := &keyring.Keyring{Project: "knxctl test", Backbone: &keyring.Backbone{MulticastAddress: "224.0.23.12", Key: keyring.Key{7}},
		Interfaces: []keyring.Interface{
			{Type: keyring.Tunneling, IndividualAddress: tunnelAddrs[0], Host: knx.MustParseIndividualAddress("1.1.0"), HasHost: true, UserID: 2, Password: "pw2", Authentication: "dev",
				Groups: map[knx.GroupAddress][]knx.IndividualAddress{g: {tunnelAddrs[1]}}},
			{Type: keyring.Tunneling, IndividualAddress: tunnelAddrs[1], Host: knx.MustParseIndividualAddress("1.1.0"), HasHost: true, UserID: 3, Password: "pw3", Authentication: "dev",
				Groups: map[knx.GroupAddress][]knx.IndividualAddress{g: {tunnelAddrs[0]}}},
		},
		GroupKeys: map[knx.GroupAddress]keyring.Key{g: {0x42}},
		Devices:   []keyring.Device{{IndividualAddress: knx.MustParseIndividualAddress("1.1.0"), ManagementPassword: "pw1", Authentication: "dev"}},
	}
	b, err := kr.Encode("kpw")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "test.knxkeys")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSecureTunnelFlags(t *testing.T) {
	s := startServer(t, secureOption(t))
	sec := []string{"--gateway", s.addr, "--user-id", "2", "--user-password", "pw2", "--device-password", "dev"}
	want(t, run(t, append([]string{"group", "write", "1/2/6", "19.5", "--dpt", "9.001"}, sec...)...), 0, "write 1/2/6 079E")
	want(t, run(t, append([]string{"group", "read", "1/2/4", "--dpt", "9.001"}, sec...)...), 0, "21.5")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--user-id", "2", "--user-password", "bad"), 6, "not authenticated")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--user-id", "2", "--user-password", "pw2", "--device-password", "bad"), 6, "device authentication")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--user-id", "2"), 3, "--user-password")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--user-id", "200", "--user-password", "x"), 3, "--user-id")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--tcp"), 9, "E_CONNECTION_TYPE")
	want(t, run(t, "tunnel", "connect", s.addr, "--user-id", "3", "--user-password", "pw3", "--format", "json"), 0, `"individualAddress": "`+tunnelAddrs[1].String()+`"`)
	want(t, run(t, "property", "read", s.addr, "knxnetip", "76", "--count", "11", "--user-id", "1", "--user-password", "pw1"), 0, `"knxctl test"`)
	want(t, run(t, "property", "read", s.addr, "knxnetip", "76", "--user-id", "2", "--user-password", "pw2"), 9, "E_CONNECTION_TYPE")
}

func TestKeyringFlags(t *testing.T) {
	s := startServer(t, secureOption(t))
	kr := testKeyring(t)
	link := []string{"--gateway", s.addr, "--keyring", kr, "--keyring-password", "kpw"}
	// The keyring's first user (2), then user 3 by --user-id and by its
	// tunnelling address.
	want(t, run(t, "tunnel", "connect", s.addr, "--keyring", kr, "--keyring-password", "kpw", "--format", "json"), 0, tunnelAddrs[0].String())
	want(t, run(t, "tunnel", "connect", s.addr, "--keyring", kr, "--keyring-password", "kpw", "--user-id", "3"), 0, tunnelAddrs[1].String())
	want(t, run(t, "tunnel", "connect", s.addr, "--keyring", kr, "--keyring-password", "kpw", "--tunnel-address", tunnelAddrs[1].String()), 0, tunnelAddrs[1].String())
	want(t, run(t, "tunnel", "connect", s.addr, "--keyring", kr, "--keyring-password", "kpw", "--user-id", "9"), 3, "not in the keyring")
	want(t, run(t, "property", "read", s.addr, "knxnetip", "76", "--count", "11", "--keyring", kr, "--keyring-password", "kpw", "--user-id", "1"), 0, `"knxctl test"`)
	want(t, run(t, "tunnel", "connect", s.addr, "--keyring", kr, "--keyring-password", "kpw", "--tunnel-address", "1.1.99"), 3, "no secure tunnelling interface")
	want(t, run(t, "tunnel", "connect", s.addr, "--keyring", kr, "--keyring-password", "wrong"), 3, "signature")

	// Data Secure: user 3 listens, user 2 writes; the server sees only
	// S-A_Data.
	l := start(t, append([]string{"group", "listen", "1/2/9", "-n", "1", "--dpt", "9.001", "--user-id", "3"}, link...)...)
	l.waitErr(t, "listening")
	want(t, run(t, append([]string{"group", "write", "1/2/9", "22.5", "--dpt", "9.001"}, link...)...), 0, "write 1/2/9")
	want(t, l.wait(t), 0, "22.5", "[secure]")
	g := knx.MustParseGroupAddress("1/2/9")
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, tg := range s.telegrams {
		if gg, ok := tg.Frame.Group(); ok && gg == g {
			n++
			if !datasecure.IsSecure(tg.Frame) {
				t.Errorf("the server saw a plain telegram to 1/2/9: %s", tg.Frame)
			}
		}
	}
	if n == 0 {
		t.Error("the server saw no telegram to 1/2/9")
	}
}

// freeBothPort returns a port free for UDP and TCP on 127.0.0.1.
func freeBothPort(t *testing.T) string {
	t.Helper()
	for i := 0; i < 50; i++ {
		u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		port := u.LocalAddr().(*net.UDPAddr).Port
		l, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		_ = u.Close()
		if err == nil {
			_ = l.Close()
			return fmt.Sprintf("127.0.0.1:%d", port)
		}
	}
	t.Fatal("no port free for UDP and TCP")
	return ""
}

func TestServeSecure(t *testing.T) {
	b := start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--tunnels", "1.1.10,1.1.11", "--secure-user", "2:pw2:1.1.10", "--device-password", "dev", "--object", "1/2/3:1.001")
	b.waitErr(t, "serving on")
	addr := regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	want(t, run(t, "group", "write", "1/2/3", "on", "--gateway", addr, "--user-id", "2", "--user-password", "pw2", "--device-password", "dev"), 0)
	want(t, run(t, "group", "write", "1/2/3", "on", "--gateway", addr), 9, "E_CONNECTION_TYPE")
	b.stop(t)

	kr := testKeyring(t)
	b = start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--keyring", kr, "--keyring-password", "kpw", "--object", "1/2/3:1.001")
	b.waitErr(t, "serving on")
	addr = regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	want(t, run(t, "group", "write", "1/2/9", "21", "--dpt", "9.001", "--gateway", addr, "--keyring", kr, "--keyring-password", "kpw"), 0)
	eventually(t, "the decrypted telegram", func() bool { return regexp.MustCompile(`1/2/9 .*write.*0C1A \[secure\]`).MatchString(b.out.String()) })
	want(t, run(t, "property", "read", addr, "device", "11", "--keyring", kr, "--keyring-password", "kpw", "--user-id", "1"), 0)
	b.stop(t)

	for _, args := range [][]string{
		{"--secure-user", "2"},
		{"--secure-user", "x:pw"},
		{"--secure-user", "2:pw:9.9.x"},
		{"--secure-user", "2:pw"},
		{"--secure-user", "2:pw€", "--device-password", "d"},
		{"--secure-user", "2:pw", "--device-password", "d€"},
		{"--keyring", kr, "--keyring-password", "kpw", "--keyring-host", "x"},
	} {
		r := run(t, append([]string{"serve", "--listen", "127.0.0.1:0", "--no-discovery"}, args...)...)
		if r.code != 3 {
			t.Errorf("%v: exit %d %s", args, r.code, r.errOut)
		}
	}
}

// TestSecureUDP: a secure session over UDP, and its flag errors.
func TestSecureUDP(t *testing.T) {
	s := startServer(t, secureOption(t))
	sec := []string{"--gateway", s.addr, "--user-id", "2", "--user-password", "pw2", "--device-password", "dev", "--udp"}
	want(t, run(t, append([]string{"group", "write", "1/2/6", "19.5", "--dpt", "9.001"}, sec...)...), 0, "write 1/2/6 079E")
	want(t, run(t, "tunnel", "connect", s.addr, "--user-id", "3", "--user-password", "pw3", "--udp", "--format", "json"), 0, `"individualAddress": "`+tunnelAddrs[1].String()+`"`)
	want(t, run(t, append([]string{"group", "write", "1/2/6", "1", "--dpt", "9.001", "--tcp"}, sec...)...), 3, "--udp and --tcp")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--udp"), 3, "--udp is for secure sessions")
}

// TestServeSecureRoutingFlags: --backbone-key is for routing. (go-knx's
// server tests cover secure routing; serve's default group is not
// usable in tests.)
func TestServeSecureRoutingFlags(t *testing.T) {
	want(t, run(t, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--backbone-key", "000102030405060708090a0b0c0d0e0f"), 3, "needs --routing")
	want(t, run(t, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--routing", "--backbone-key", "xyz"), 3, "--backbone-key")
}

func TestKeyringShow(t *testing.T) {
	kr := testKeyring(t)
	r := run(t, "keyring", "show", kr, "--keyring-password", "kpw")
	want(t, r, 0, "interface 1.1.10 Tunneling host 1.1.0 user 2", "secure group 1/2/9", "hidden")
	if regexp.MustCompile(`pw2|4200`).MatchString(r.out) {
		t.Errorf("secrets shown: %s", r.out)
	}
	want(t, run(t, "keyring", "show", kr, "--keyring-password", "kpw", "--secrets", "--format", "json"), 0, `"password": "pw2"`, `"key": "42000000000000000000000000000000"`, `"1/2/9"`)
	want(t, run(t, "keyring", "show", kr, "--keyring-password", "x"), 3, "signature")
}

func TestSecureRouting(t *testing.T) {
	link := routingArgs(t)
	key := []string{"--backbone-key", "000102030405060708090a0b0c0d0e0f"}
	l := start(t, append(append([]string{"group", "listen", "1/2/3", "--routing", "-n", "1", "--dpt", "1.001"}, link...), key...)...)
	l.waitErr(t, "listening")
	want(t, routedWrites(t, l, append(link, key...), "1/2/3", "on"), 0, "1/2/3")
	want(t, run(t, append([]string{"group", "write", "1/2/3", "on", "--routing", "--backbone-key", "xyz"}, link...)...), 3, "--backbone-key")
}
