// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/otfabric/knxctl/internal/errs"
)

const siteConfig = `{
  "name": "test site",
  "address": "1.0.0",
  "tunnels": ["1.1.30", "1.1.31"],
  "devices": [
    {"name": "living", "address": "1.1.2", "objects": [
      {"name": "light", "dpt": "1.001", "flags": "CRWU", "groups": ["2/0/1", "2/0/2"], "value": "on"},
      {"name": "mode", "dpt": "20.102", "groups": ["2/0/3"], "value": "comfort"}
    ]}
  ]
}`

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var servingRe = regexp.MustCompile(`serving on (\S+) \((\d+) devices`)

// serve starts knxctl serve on 127.0.0.1 and returns its address.
func serve(t *testing.T, args ...string) (*background, string) {
	t.Helper()
	b := start(t, append([]string{"serve", "--listen", "127.0.0.1:0", "--no-discovery"}, args...)...)
	b.waitErr(t, "serving on ")
	m := servingRe.FindStringSubmatch(b.errOut.String())
	if m == nil {
		t.Fatalf("no address in %q", b.errOut.String())
	}
	return b, m[1]
}

func TestServe(t *testing.T) {
	cfg := writeFile(t, "site.json", siteConfig)
	b, addr := serve(t, "--config", cfg, "--object", "1/3/1:9.001:CRT=21.5", "--object", "1/3/2+1/3/3:1.001", "--device-address", "1.1.3", "--format", "json")
	if !strings.Contains(b.errOut.String(), "(2 devices") {
		t.Fatalf("stderr %q", b.errOut.String())
	}
	// The configuration: name, address, tunnelling addresses.
	res := resultOf(t, run(t, "describe", addr, "--format", "json"), "describe")
	if d := res["device"].(map[string]any); d["name"] != "test site" || d["individualAddress"] != "1.0.0" {
		t.Fatalf("device %v", d)
	}
	res = resultOf(t, run(t, "tunnel", "connect", addr, "--format", "json"), "tunnel connect")
	if res["individualAddress"] != "1.1.30" {
		t.Fatalf("tunnel %v", res)
	}
	// Objects of --object and of the configuration answer reads.
	want(t, run(t, "group", "read", "1/3/1", "--dpt", "9.001", "-g", addr), 0, "1.1.3", "(21.5 °C)")
	want(t, run(t, "group", "read", "2/0/2", "--dpt", "1.001", "-g", addr), 0, "1.1.2", "01 (on)")
	// An object without "flags" has the default flags CRWU.
	want(t, run(t, "group", "read", "2/0/3", "--dpt", "20.102", "-g", addr), 0, "(comfort)")
	// A write changes the object; the next read returns the new value.
	want(t, run(t, "group", "write", "2/0/1", "off", "-g", addr), 0)
	want(t, run(t, "group", "read", "2/0/1", "--dpt", "1.001", "-g", addr), 0, "00 (off)")
	// An object without the R flag does not answer.
	want(t, run(t, "group", "read", "1/3/2", "-g", addr, "--timeout", "300ms"), errs.ExitTimeout)
	r := b.stop(t)
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	var sawWrite, sawDecoded bool
	for _, rec := range recs {
		if rec["type"] != "telegram" {
			t.Fatalf("record %v", rec)
		}
		if rec["group"] == "2/0/1" && rec["service"] == "write" && rec["source"] == "1.1.30" && rec["text"] == "off" && rec["dpt"] == "1.001" {
			sawWrite = true
		}
		if rec["group"] == "1/3/1" && rec["service"] == "response" && rec["value"] == 21.5 {
			sawDecoded = true
		}
	}
	if !sawWrite || !sawDecoded {
		t.Fatalf("records lack the write or the decoded response:\n%s", r.out)
	}
}

func TestServeFlags(t *testing.T) {
	// Without --config: --name, --address, --tunnels; text output.
	b, addr := serve(t, "--name", "flags", "--address", "1.2.0", "--tunnels", "1.2.10, 1.2.11", "--object", "4/0/1:5.001:CRW=50")
	res := resultOf(t, run(t, "describe", addr, "--format", "json"), "describe")
	if d := res["device"].(map[string]any); d["name"] != "flags" || d["individualAddress"] != "1.2.0" {
		t.Fatalf("device %v", d)
	}
	want(t, run(t, "group", "read", "4/0/1", "-g", addr), 0, "1.1.1", "-> 4/0/1")
	r := b.stop(t)
	want(t, r, 0, "-> 4/0/1", "read", "response")
	// --name overrides the configuration's name.
	b, addr = serve(t, "--config", writeFile(t, "c.json", siteConfig), "--name", "override")
	res = resultOf(t, run(t, "describe", addr, "--format", "json"), "describe")
	if d := res["device"].(map[string]any); d["name"] != "override" {
		t.Fatalf("device %v", d)
	}
	want(t, b.stop(t), 0)
}

func TestServeInvalid(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"--config", filepath.Join(dir, "missing.json")}, "--config"},
		{[]string{"--config", writeFile(t, "bad.json", "{")}, "--config"},
		{[]string{"--object", "1/2/3"}, "want group"},
		{[]string{"--object", "1/2/3:1.001:CRWU:x"}, "want group"},
		{[]string{"--object", "1/2/3:nope"}, "object"},
		{[]string{"--object", "1/2/3:1.001:XYZ"}, "object"},
		{[]string{"--object", "1/2/x:1.001"}, "object"},
		{[]string{"--object", "1/2/3:9.001=warm"}, "object"},
		{[]string{"--object", "1/2/3:9.001=1e9"}, "object"},
		{[]string{"--object", "1/2/3:1.001", "--device-address", "x"}, "device"},
		{[]string{"--address", "x"}, "--address"},
		{[]string{"--tunnels", "1.1.1,x"}, "--tunnels"},
		{[]string{"--tunnels", "1.1.1,1.1.1"}, "duplicate"},
		{[]string{"--name", strings.Repeat("n", 31)}, "at most 30"},
		{[]string{"--routing"}, "routing"},
		{[]string{"--interface", "no-such-if0"}, "no-such-if0"},
	}
	for _, c := range cases {
		args := append([]string{"serve", "--listen", "127.0.0.1:0", "--no-discovery"}, c.args...)
		r := run(t, args...)
		want(t, r, errs.ExitInvalidInput, c.msg)
	}
	r := run(t, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--object", "1/2/3", "--format", "json")
	want(t, r, errs.ExitInvalidInput)
	if e := errorOf(t, r); e["kind"] != "invalid_input" {
		t.Fatalf("error %v", e)
	}
	// The port is taken.
	s := startServer(t)
	r = run(t, "serve", "--listen", s.addr, "--no-discovery")
	want(t, r, errs.ExitTransport, "listen")
}
