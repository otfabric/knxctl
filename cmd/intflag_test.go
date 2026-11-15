// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/otfabric/go-knx/knxnetip"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// intBoundaries are the values of the boundary tables: around zero, the
// documented limits of the flags, and the 32-bit and 64-bit boundaries.
var intBoundaries = []string{
	"-1", "0", "1", "2", "3", "4", "127", "128", "65535", "65536", "0x10",
	"2147483647", "2147483648", "-2147483648", "-2147483649",
	"4294967295", "4294967296", "4294967297", "4294967298", "4294967305",
	"9223372036854775807", "9223372036854775808", "-9223372036854775809",
}

// fitsInt parses s as the platform's int; ok is false when it cannot hold it.
func fitsInt(s string) (int64, bool) {
	v, err := strconv.ParseInt(s, 0, 64)
	return v, err == nil && v >= math.MinInt && v <= math.MaxInt
}

// Every int flag holds exactly the value given, or refuses it when the
// platform's int cannot hold it; the usage keeps the type name int.
func TestIntFlagsHoldTheValueGiven(t *testing.T) {
	names := map[string]bool{}
	visitFlags(func(c *cobra.Command, fl *pflag.Flag) {
		if fl.Value.Type() != "int" {
			return
		}
		names[fl.Name] = true
		if typ := fmt.Sprintf("%T", fl.Value); typ != "cmd.intValue" {
			t.Errorf("%s --%s is a %s, not a cmd.intValue", c.CommandPath(), fl.Name, typ)
		}
	})
	if len(names) != 4 || !names["count"] || !names["main"] || !names["repeat"] || !names["user-id"] {
		t.Fatalf("int flags %v, want count, main, repeat and user-id", names)
	}
	for _, s := range intBoundaries {
		visitFlags(func(c *cobra.Command, fl *pflag.Flag) {
			if fl.Value.Type() != "int" {
				return
			}
			err := fl.Value.Set(s)
			v, fits := fitsInt(s)
			switch {
			case fits && err != nil:
				t.Errorf("%s --%s %s: %v", c.CommandPath(), fl.Name, s, err)
			case fits && fl.Value.String() != strconv.FormatInt(v, 10):
				t.Errorf("%s --%s %s holds %s", c.CommandPath(), fl.Name, s, fl.Value.String())
			case !fits && err == nil:
				t.Errorf("%s --%s %s accepted as %s; the int of this platform cannot hold it", c.CommandPath(), fl.Name, s, fl.Value.String())
			}
		})
	}
}

// A value outside a flag's range, or one the platform's int cannot hold,
// ends the command with exit 2 or 3 naming the value given, before
// anything is sent; never with another value.
func TestIntFlagBoundaries(t *testing.T) {
	p := startHostile(t, func(*hostilePeer, netip.AddrPort, knxnetip.Message) {})
	timeout := []string{"--timeout", "2s"}
	cases := []struct {
		args   []string
		lo, hi int64
	}{
		{[]string{"dpt", "list", "--main"}, 0, 0xFFFF},
		{append([]string{"discover", "--unicast", p.addr}, append(timeout, "--repeat")...), 1, 3},
		{append([]string{"group", "write", "1/2/3", "on", "-g", p.addr, "--udp", "--user-password", "pw"}, append(timeout, "--user-id")...), 0, 127},
		{append([]string{"monitor", "-g", p.addr}, append(timeout, "--count")...), 0, math.MaxInt},
		{append([]string{"group", "listen", "-g", p.addr}, append(timeout, "--count")...), 0, math.MaxInt},
		{append([]string{"group", "answer", "1/2/3", "on", "-g", p.addr}, append(timeout, "--count")...), 0, math.MaxInt},
		{append([]string{"busmonitor", p.addr}, append(timeout, "--count")...), 0, math.MaxInt},
	}
	for _, c := range cases {
		name := strings.Join(c.args, " ")
		for _, s := range intBoundaries {
			if v, fits := fitsInt(s); fits && v >= c.lo && v <= c.hi {
				if c.args[0] == "dpt" {
					// Offline: the value is used as given.
					want(t, run(t, append(c.args, s)...), 0)
				}
				continue
			}
			r := run(t, append(c.args, s)...)
			if r.code != 2 && r.code != 3 {
				t.Errorf("%s %s: exit %d, want 2 or 3\nstdout: %s\nstderr: %s", name, s, r.code, r.out, r.errOut)
				continue
			}
			if v, err := strconv.ParseInt(s, 0, 64); !strings.Contains(r.errOut, s) && (err != nil || !strings.Contains(r.errOut, " "+strconv.FormatInt(v, 10)+" ")) {
				t.Errorf("%s %s: the error does not name the value given: %s", name, s, r.errOut)
			}
		}
	}
	// --main 9 lists the subtypes of main number 9 only.
	r := run(t, "dpt", "list", "--main", "9")
	want(t, r, 0, "9.001")
	if strings.Contains(r.out, "\n1.001") || strings.HasPrefix(r.out, "1.001") {
		t.Errorf("dpt list --main 9 lists other main numbers:\n%s", r.out)
	}
	if n := p.got.Load(); n != 0 {
		t.Fatalf("%d datagrams sent", n)
	}
}
