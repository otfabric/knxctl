// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/otfabric/go-knx/datasecure"
	"github.com/otfabric/go-knx/server"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// secretVars are the variables of the secret flags.
var secretVars = []string{"KNXCTL_KEYRING_PASSWORD", "KNXCTL_USER_PASSWORD", "KNXCTL_DEVICE_PASSWORD", "KNXCTL_BACKBONE_KEY", "KNXCTL_TOOL_KEY"}

// commandPaths returns the arguments of every command of the tree.
func commandPaths() [][]string {
	var paths [][]string
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		paths = append(paths, path)
		for _, sub := range c.Commands() {
			walk(sub, append(append([]string{}, path...), sub.Name()))
		}
	}
	walk(newRoot(&app{}), nil)
	return paths
}

// The usage text of every command holds no value of a secret variable:
// the secret flags have no default value, and the environment is read
// after the command line.
func TestUsageHasNoSecretValues(t *testing.T) {
	markers := map[string]string{}
	for i, v := range secretVars {
		markers[v] = "marker" + strings.Repeat("Q", i+1) + "value"
		t.Setenv(v, markers[v])
	}
	n := 0
	for _, path := range commandPaths() {
		r := run(t, append(path, "--help")...)
		if r.code != 0 {
			t.Fatalf("knxctl %s --help: exit %d\n%s", strings.Join(path, " "), r.code, r.errOut)
		}
		for v, m := range markers {
			if strings.Contains(r.out+r.errOut, m) {
				t.Errorf("knxctl %s --help holds the value of %s", strings.Join(path, " "), v)
			}
		}
		n++
	}
	if n < 30 {
		t.Fatalf("%d commands", n)
	}
}

// docs/cli.md, the usage text of every command, gives no secret flag a
// default value.
func TestCLIReferenceHasNoSecretDefaults(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	names := map[string]bool{}
	visitFlags(func(_ *cobra.Command, f *pflag.Flag) {
		if _, ok := f.Annotations[secretAnnotation]; ok {
			names[f.Name] = true
		}
	})
	if len(names) != 5 {
		t.Fatalf("secret flags %v", names)
	}
	seen := 0
	for line := range strings.SplitSeq(doc, "\n") {
		for name := range names {
			if regexp.MustCompile(`^\s+--` + regexp.QuoteMeta(name) + `\s`).MatchString(line) {
				seen++
				if strings.Contains(line, `(default "`) {
					t.Errorf("docs/cli.md: a default for --%s: %s", name, line)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("docs/cli.md lists no secret flag")
	}
}

// visitFlags calls f for every local flag of every command.
func visitFlags(f func(*cobra.Command, *pflag.Flag)) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(fl *pflag.Flag) { f(c, fl) })
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRoot(&app{}))
}

// Every secret flag has a -file twin and a variable, and its usage marks
// the flag as deprecated; so does --secure-user's.
func TestSecretFlagsHaveFiles(t *testing.T) {
	secretNames := map[string]bool{"keyring-password": true, "user-password": true, "device-password": true, "backbone-key": true, "tool-key": true}
	n := 0
	visitFlags(func(c *cobra.Command, f *pflag.Flag) {
		ann, ok := f.Annotations[secretAnnotation]
		if secretNames[f.Name] != ok {
			t.Errorf("%s --%s: secret %v, want %v", c.CommandPath(), f.Name, ok, secretNames[f.Name])
		}
		if !ok {
			return
		}
		n++
		if !strings.Contains(f.Usage, "deprecated as a flag") || !strings.Contains(f.Usage, "--"+f.Name+"-file") {
			t.Errorf("%s --%s: usage %q", c.CommandPath(), f.Name, f.Usage)
		}
		file := c.LocalFlags().Lookup(f.Name + "-file")
		if file == nil || ann[1] != file.Name || !strings.Contains(file.Usage, "env "+ann[0]+"_FILE") {
			t.Errorf("%s --%s: no -file flag with %s_FILE", c.CommandPath(), f.Name, ann[0])
		}
		if f.DefValue != "" {
			t.Errorf("%s --%s: default %q", c.CommandPath(), f.Name, f.DefValue)
		}
	})
	if n < 20 {
		t.Fatalf("%d secret flags", n)
	}
	serve := newServeCmd(&app{})
	if u := serve.Flags().Lookup("secure-user").Usage; !strings.Contains(u, "deprecated: use --secure-users-file") {
		t.Errorf("--secure-user: usage %q", u)
	}
}

// secretFile writes content to a file of its own.
func secretFile(t *testing.T, content string) string {
	t.Helper()
	return writeFile(t, "secret", content)
}

// The -file flags and the _FILE variables give the secret like the flag:
// the trailing newline (LF or CRLF) is not part of it.
func TestSecretFiles(t *testing.T) {
	s := startServer(t, secureOption(t))
	pw2, dev := secretFile(t, "pw2\n"), secretFile(t, "dev\r\n")
	want(t, run(t, "group", "write", "1/2/6", "19.5", "--dpt", "9.001", "--gateway", s.addr, "--user-id", "2", "--user-password-file", pw2, "--device-password-file", dev), 0, "write 1/2/6 079E")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--user-id", "2", "--user-password-file", pw2, "--device-password-file", secretFile(t, "bad\n")), 6, "device authentication")
	want(t, run(t, "group", "write", "1/2/6", "1", "--dpt", "9.001", "--gateway", s.addr, "--user-id", "2", "--user-password-file", secretFile(t, "pw2\n\n")), 6, "not authenticated")

	t.Setenv("KNXCTL_USER_PASSWORD_FILE", pw2)
	t.Setenv("KNXCTL_DEVICE_PASSWORD_FILE", dev)
	want(t, run(t, "tunnel", "connect", s.addr, "--user-id", "2", "--format", "json"), 0, `"individualAddress": "`+tunnelAddrs[0].String()+`"`)
	// The command line wins over the variables.
	want(t, run(t, "tunnel", "connect", s.addr, "--user-id", "3", "--user-password", "pw3", "--format", "json"), 0, `"individualAddress": "`+tunnelAddrs[1].String()+`"`)
	want(t, run(t, "tunnel", "connect", s.addr, "--user-id", "3", "--user-password-file", secretFile(t, "pw3"), "--format", "json"), 0, `"individualAddress": "`+tunnelAddrs[1].String()+`"`)
	t.Setenv("KNXCTL_USER_PASSWORD_FILE", "")
	t.Setenv("KNXCTL_DEVICE_PASSWORD_FILE", "")

	// The variables without _FILE, as before.
	t.Setenv("KNXCTL_USER_PASSWORD", "pw2")
	t.Setenv("KNXCTL_DEVICE_PASSWORD", "dev")
	want(t, run(t, "diagnose", s.addr, "--user-id", "2"), 0, "secured services")
	t.Setenv("KNXCTL_USER_PASSWORD", "")
	t.Setenv("KNXCTL_DEVICE_PASSWORD", "")

	kr, kpw := testKeyring(t), secretFile(t, "kpw\n")
	want(t, run(t, "tunnel", "connect", s.addr, "--keyring", kr, "--keyring-password-file", kpw), 0, tunnelAddrs[0].String())
	want(t, run(t, "keyring", "show", kr, "--keyring-password-file", kpw), 0, "secure group 1/2/9")
	t.Setenv("KNXCTL_KEYRING_PASSWORD_FILE", kpw)
	want(t, run(t, "keyring", "show", kr), 0, "secure group 1/2/9")
	t.Setenv("KNXCTL_KEYRING_PASSWORD_FILE", "")

	// The backbone key is read (it needs --routing) and checked.
	key := secretFile(t, "000102030405060708090a0b0c0d0e0f\n")
	want(t, run(t, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--backbone-key-file", key), 3, "--backbone-key needs --routing")
	want(t, run(t, "group", "write", "1/2/3", "on", "--routing", "--backbone-key-file", secretFile(t, "xyz\n")), 3, "--backbone-key: want 32 hex digits")

	// The tool key of remote device management, and of serve.
	var tk datasecure.Key
	copy(tk[:], "tool-key-for-tes")
	sec := startServer(t, server.WithIndividualAddress(hostIA), server.WithToolKey(tk))
	tool := secretFile(t, toolKeyHex+"\n")
	want(t, run(t, "property", "read", sec.addr, "device", "11", "--device", "1.1.0", "--tool-key-file", tool), 0, "[tool access]")
	b := start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--address", "1.1.0", "--tool-key-file", tool)
	b.waitErr(t, "serving on")
	addr := regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	want(t, run(t, "property", "read", addr, "device", "11", "--device", "1.1.0", "--tool-key-file", tool), 0, "[tool access]")
	want(t, b.stop(t), 0)
}

// A secret file that cannot be read, is empty or too large, or that
// competes with its flag or variable, is refused (exit 3) before anything
// is sent.
func TestSecretFileErrors(t *testing.T) {
	a := &acceptor{}
	p := startHostile(t, a.handle)
	big := filepath.Join(t.TempDir(), "big")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxSecretFileSize + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	missing := filepath.Join(t.TempDir(), "missing")
	write := []string{"group", "write", "1/2/3", "on", "-g", p.addr, "--user-id", "2"}
	cases := []struct {
		env  map[string]string
		args []string
		msg  string
	}{
		{nil, append(write, "--user-password-file", missing), "--user-password-file"},
		{nil, append(write, "--user-password-file", big), "larger than 16 MiB"},
		{nil, append(write, "--user-password-file", secretFile(t, "\n")), "empty"},
		{nil, append(write, "--user-password", "pw2", "--user-password-file", secretFile(t, "pw2")), "exclude each other"},
		{nil, append(write, "--user-password", "pw2", "--device-password-file", missing), "--device-password-file"},
		{map[string]string{"KNXCTL_USER_PASSWORD_FILE": missing}, write, "KNXCTL_USER_PASSWORD_FILE"},
		{map[string]string{"KNXCTL_USER_PASSWORD": "pw2", "KNXCTL_USER_PASSWORD_FILE": secretFile(t, "pw2")}, write, "exclude each other"},
		{nil, []string{"monitor", "-g", p.addr, "--keyring-password-file", missing}, "--keyring-password-file"},
		{nil, []string{"property", "read", p.addr, "device", "11", "--device", "1.1.5", "--tool-key-file", missing}, "--tool-key-file"},
		{nil, []string{"keyring", "show", testKeyring(t), "--keyring-password-file", big}, "larger than"},
		{nil, []string{"serve", "--listen", "127.0.0.1:0", "--no-discovery", "--device-password-file", missing}, "--device-password-file"},
		{nil, []string{"serve", "--listen", "127.0.0.1:0", "--no-discovery", "--secure-users-file", big}, "larger than"},
		{map[string]string{"KNXCTL_SECURE_USERS_FILE": missing}, []string{"serve", "--listen", "127.0.0.1:0", "--no-discovery"}, "KNXCTL_SECURE_USERS_FILE"},
	}
	for _, c := range cases {
		for k, v := range c.env {
			t.Setenv(k, v)
		}
		want(t, run(t, c.args...), 3, c.msg)
		for k := range c.env {
			t.Setenv(k, "")
		}
	}
	if n := p.got.Load(); n != 0 {
		t.Fatalf("%d datagrams sent", n)
	}
}

// serve reads its secure users from a file, one per line as --secure-user
// takes them; an entry that is wrong is named by its line, the password
// masked.
func TestServeSecureUsersFile(t *testing.T) {
	users := writeFile(t, "users", "# secure tunnelling users\n2:pw2:1.1.10\n\n3:pw3:1.1.11\r\n")
	dev := secretFile(t, "dev\n")
	b := start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--tunnels", "1.1.10,1.1.11", "--secure-users-file", users, "--device-password-file", dev, "--object", "1/2/3:1.001")
	b.waitErr(t, "serving on")
	addr := regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	want(t, run(t, "tunnel", "connect", addr, "--user-id", "2", "--user-password", "pw2", "--device-password", "dev", "--format", "json"), 0, `"individualAddress": "1.1.10"`)
	want(t, run(t, "tunnel", "connect", addr, "--user-id", "3", "--user-password", "pw3", "--device-password", "dev", "--format", "json"), 0, `"individualAddress": "1.1.11"`)
	want(t, run(t, "group", "write", "1/2/3", "on", "--gateway", addr), 6, "E_CONNECTION_TYPE")
	want(t, b.stop(t), 0)

	// The variable, and --secure-user beside the file.
	t.Setenv("KNXCTL_SECURE_USERS_FILE", writeFile(t, "users", "2:pw2:1.1.10\n"))
	b = start(t, "serve", "--listen", freeBothPort(t), "--no-discovery", "--tunnels", "1.1.10,1.1.11", "--secure-user", "3:pw3:1.1.11", "--device-password-file", dev)
	b.waitErr(t, "serving on")
	addr = regexp.MustCompile(`serving on (\S+)`).FindStringSubmatch(b.errOut.String())[1]
	want(t, run(t, "tunnel", "connect", addr, "--user-id", "2", "--user-password", "pw2", "--device-password", "dev"), 0)
	want(t, run(t, "tunnel", "connect", addr, "--user-id", "3", "--user-password", "pw3", "--device-password", "dev"), 0)
	want(t, b.stop(t), 0)
	t.Setenv("KNXCTL_SECURE_USERS_FILE", "")

	for _, c := range []struct{ content, msg string }{
		{"2:pw2:1.1.10\nsecretpw\n", "line 2: want id:password"},
		{"x:secretpw\n", `line 1: user id "x"`},
		{"2:secretpw:9.9.x\n", "line 1 2:"},
		{"2:secretpw\u20ac\n", "line 1 2:"},
	} {
		r := run(t, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--secure-users-file", writeFile(t, "users", c.content))
		want(t, r, 3, c.msg)
		if strings.Contains(r.out+r.errOut, "secretpw") {
			t.Errorf("%q: the password in %s", c.content, r.errOut)
		}
	}
	// Comments and empty lines only: no user, so not a secure server.
	want(t, run(t, "serve", "--listen", "127.0.0.1:0", "--no-discovery", "--secure-users-file", writeFile(t, "users", "# none\n\n\r\n"), "--secure-session-timeout", "1m"), 3, "needs a secure server")
}
