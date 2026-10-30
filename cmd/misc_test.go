// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/otfabric/knxctl/internal/errs"
)

func TestVersion(t *testing.T) {
	SetBuildMeta("", "", "", "")
	want(t, run(t, "version"), 0, "knxctl dev", "go-knx")
	SetBuildMeta("v0.1.0", "v0.1.0", "abc1234", "2026-10-29T00:00:00Z")
	defer SetBuildMeta("", "", "", "")
	r := run(t, "version")
	want(t, r, 0, "knxctl v0.1.0 (abc1234)")
	r = run(t, "version", "--format", "json")
	res := resultOf(t, r, "version")
	if res["version"] != "v0.1.0" || res["commit"] != "abc1234" || res["tag"] != "v0.1.0" || res["buildDate"] == nil || res["go"] == nil || res["platform"] == nil || res["goKnx"] == nil {
		t.Fatalf("version %v", res)
	}
	// A table falls back to text for results without columns.
	want(t, run(t, "version", "--format", "table"), 0, "knxctl v0.1.0")
}

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
	// Flag value completion of --format.
	want(t, run(t, "__complete", "--format", ""), 0, "json", "table", "text")
}

func TestHelp(t *testing.T) {
	r := run(t, "--help")
	want(t, r, 0, "Passive by default", "group", "serve", "monitor")
	want(t, run(t, "group", "write", "--help"), 0, "--dpt", "--raw", "--small")
}

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"nope"}, "unknown command"},
		{[]string{"version", "--nope"}, "unknown flag"},
		{[]string{"version", "extra"}, "unknown command"},
		{[]string{"describe"}, "accepts 1 arg"},
		{[]string{"group", "read"}, "accepts 1 arg"},
		{[]string{"group", "read", "1/2/3", "--timeout"}, "flag needs an argument"},
		{[]string{"group", "read", "1/2/3", "--timeout", "soon"}, "invalid argument"},
		{[]string{"group", "write", "1/2/3", "on"}, "--gateway host"},
		{[]string{"group", "write", "1/2/3", "-g", "127.0.0.1:1"}, "a value or --raw is required"},
		{[]string{"group", "answer", "1/2/3", "-g", "127.0.0.1:1"}, "a value or --raw is required"},
		{[]string{"monitor"}, "--gateway host"},
	}
	for _, c := range cases {
		r := run(t, c.args...)
		want(t, r, errs.ExitUsage, c.msg)
		if !strings.Contains(r.errOut, "Run 'knxctl --help' for usage.") || r.out != "" {
			t.Fatalf("%v: stdout %q, stderr %q", c.args, r.out, r.errOut)
		}
		// With --format json the error is one document on stdout.
		r = run(t, append(c.args, "--format", "json")...)
		if c.args[len(c.args)-1] == "--timeout" {
			continue // --format json would be the missing value
		}
		want(t, r, errs.ExitUsage)
		e := errorOf(t, r)
		if e["kind"] != "usage" || e["code"] != "invalid_input" || !strings.Contains(e["message"].(string), c.msg) || r.errOut != "" {
			t.Fatalf("%v: error %v, stderr %q", c.args, e, r.errOut)
		}
	}
}

func TestFormatSelection(t *testing.T) {
	want(t, run(t, "version", "--format", "xml"), errs.ExitInvalidInput, "invalid --format")
	want(t, run(t, "version", "--timeout", "0s"), errs.ExitInvalidInput, "must be positive")
	// The environment selects the format; --format overrides it.
	t.Setenv("KNXCTL_FORMAT", "json")
	resultOf(t, run(t, "version"), "version")
	want(t, run(t, "version", "--format=text"), 0, "knxctl")
	// A usage error follows the environment's format too.
	errorOf(t, run(t, "nope"))
	// Arguments after -- are not flags, not even --format.
	t.Setenv("KNXCTL_FORMAT", "")
	r := run(t, "dpt", "encode", "9.001", "--", "1", "--format", "json")
	want(t, r, errs.ExitUsage, "accepts 2 arg(s)")
	if r.out != "" {
		t.Fatalf("stdout %q", r.out)
	}
}

func TestNegativeValueHint(t *testing.T) {
	r := run(t, "group", "write", "--dpt", "9.001", "-g", "127.0.0.1:1", "1/2/4", "-30")
	want(t, r, errs.ExitUsage, "unknown shorthand flag", "put -- before the arguments")
	r = run(t, "dpt", "encode", "9.001", "-.5")
	want(t, r, errs.ExitUsage, "put -- before the arguments")
	// Not for an unknown shorthand flag that is not a number.
	r = run(t, "dpt", "list", "-x")
	want(t, r, errs.ExitUsage, "unknown shorthand flag")
	if strings.Contains(r.errOut, "negative value") {
		t.Fatalf("hint without a negative value: %s", r.errOut)
	}
	// With -- the negative value is an argument.
	want(t, run(t, "dpt", "encode", "9.001", "--", "-30"), 0, "-30 °C")
}

func TestVerbosityFlags(t *testing.T) {
	for _, f := range []string{"-v", "-vv", "-q"} {
		want(t, run(t, "dpt", "describe", "1.001", f), 0, "DPT_Switch")
	}
}

// KNXCTL_OUTPUT_FORMAT is accepted; KNXCTL_FORMAT is canonical and wins.
func TestOutputFormatEnv(t *testing.T) {
	t.Setenv("KNXCTL_FORMAT", "")
	t.Setenv("KNXCTL_OUTPUT_FORMAT", "json")
	if r := run(t, "version"); !strings.Contains(r.out, `"schemaVersion"`) {
		t.Fatalf("OUTPUT_FORMAT ignored: %q", r.out)
	}
	t.Setenv("KNXCTL_FORMAT", "text")
	if r := run(t, "version"); strings.Contains(r.out, `"schemaVersion"`) {
		t.Fatalf("FORMAT does not win: %q", r.out)
	}
}
