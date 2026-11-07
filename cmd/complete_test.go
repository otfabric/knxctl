// SPDX-License-Identifier: MIT

package cmd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCompletion(t *testing.T) {
	for shell, marker := range map[string]string{
		"bash":       "__start_knxctl",
		"zsh":        "#compdef knxctl",
		"fish":       "complete -c knxctl",
		"powershell": "Register-ArgumentCompleter",
	} {
		r := run(t, "completion", shell)
		want(t, r, 0, marker)
	}
	want(t, run(t, "completion", "tcsh"), 3, "unknown shell")
	want(t, run(t, "completion"), 2, "accepts 1 arg")
}

// complete runs the hidden __complete command and returns the candidates
// (descriptions cut off) and the directive line.
func complete(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	r := run(t, append([]string{"__complete"}, args...)...)
	if r.code != 0 {
		t.Fatalf("__complete %v: exit %d\n%s%s", args, r.code, r.out, r.errOut)
	}
	lines := strings.Split(strings.TrimSuffix(r.out, "\n"), "\n")
	var got []string
	for _, l := range lines[:len(lines)-1] {
		got = append(got, strings.SplitN(l, "\t", 2)[0])
	}
	return got, lines[len(lines)-1]
}

const (
	noFileComp = ":4"
	fileExt    = ":8"
)

func TestCompleteValues(t *testing.T) {
	cases := []struct {
		args      []string
		want      []string
		directive string
	}{
		{[]string{"completion", ""}, []string{"bash", "zsh", "fish", "powershell"}, noFileComp},
		// Global flag.
		{[]string{"--format", ""}, []string{"text", "json", "table"}, noFileComp},
		{[]string{"dpt", "list", "--format", ""}, []string{"text", "json", "table"}, noFileComp},
		// group, monitor, routing.
		{[]string{"group", "write", "--priority", ""}, []string{"low", "normal", "urgent", "system"}, noFileComp},
		{[]string{"group", "answer", "--priority", ""}, []string{"low", "normal", "urgent", "system"}, noFileComp},
		{[]string{"group", "read", "--dpt", ""}, []string{"1.001", "9.001", "232.600"}, noFileComp},
		{[]string{"monitor", "-t", ""}, []string{"9.001"}, noFileComp},
		{[]string{"routing", "monitor", "--dpt", ""}, []string{"9.001"}, noFileComp},
		// discover.
		{[]string{"discover", "--service", ""}, []string{"core", "devmgmt", "tunnelling", "routing", "security"}, noFileComp},
		// dpt.
		{[]string{"dpt", "describe", ""}, []string{"1.001", "9.001"}, noFileComp},
		{[]string{"dpt", "encode", ""}, []string{"9.001"}, noFileComp},
		{[]string{"dpt", "decode", "9.001", ""}, nil, noFileComp},
		// tunnel.
		{[]string{"tunnel", "feature", "get", "host", ""}, []string{"max-apdu", "bus-connection", "info-service"}, noFileComp},
		{[]string{"tunnel", "connect", "--keyring", ""}, []string{"knxkeys"}, fileExt},
		// property.
		{[]string{"property", "read", "host", ""}, []string{"device", "router", "knxnetip"}, noFileComp},
		{[]string{"property", "write", "--keyring", ""}, []string{"knxkeys"}, fileExt},
		// keyring.
		{[]string{"keyring", "show", ""}, []string{"knxkeys"}, fileExt},
		// serve.
		{[]string{"serve", "--config", ""}, []string{"json"}, fileExt},
		{[]string{"serve", "--keyring", ""}, []string{"knxkeys"}, fileExt},
		{[]string{"serve", "--listen", ""}, nil, noFileComp},
		// Free values complete no file names.
		{[]string{"describe", ""}, nil, noFileComp},
		{[]string{"--timeout", ""}, nil, noFileComp},
		{[]string{"group", "write", "--gateway", ""}, nil, noFileComp},
	}
	for _, c := range cases {
		got, directive := complete(t, c.args...)
		if directive != c.directive {
			t.Errorf("%v: directive %s, want %s", c.args, directive, c.directive)
		}
		if c.want == nil && len(got) != 0 {
			t.Errorf("%v: %v, want nothing", c.args, got)
		}
		for _, w := range c.want {
			if !contains(got, w) {
				t.Errorf("%v: %v lacks %q", c.args, got, w)
			}
		}
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// valueSet recognises a usage that lists the values of a flag:
// "text, json or table", "low, normal, urgent or system", "a|b", "one of".
var valueSet = regexp.MustCompile(`\w+, \w+(, \w+)* or \w+|\w\|\w|one of`)

// Every flag that takes a value completes something (or explicitly no file
// names), and a flag whose usage lists a value set completes values.
func TestEveryFlagCompletes(t *testing.T) {
	root := newRoot(&app{})
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "bool" || f.Value.Type() == "count" {
				return
			}
			fn, ok := c.GetFlagCompletionFunc(f.Name)
			if !ok {
				t.Errorf("%s --%s: no completion", c.CommandPath(), f.Name)
				return
			}
			if valueSet.MatchString(f.Usage) {
				if got, _ := fn(c, nil, ""); len(got) == 0 {
					t.Errorf("%s --%s lists values (%q) but completes none", c.CommandPath(), f.Name, f.Usage)
				}
			}
		})
		if c.Runnable() && !c.HasSubCommands() && c.ValidArgsFunction == nil && len(c.ValidArgs) == 0 {
			t.Errorf("%s: no argument completion", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
