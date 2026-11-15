// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// readDoc returns a document of the repository with its whitespace
// collapsed (CRLF checkouts and line breaks do not matter).
func readDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(strings.ReplaceAll(string(b), "\r\n", "\n")), " ")
}

// between returns the part of s from start up to end (exclusive).
func between(t *testing.T, s, start, end string) string {
	t.Helper()
	i := strings.Index(s, start)
	if i < 0 {
		t.Fatalf("no %q", start)
	}
	s = s[i:]
	if j := strings.Index(s[len(start):], end); j >= 0 {
		s = s[:len(start)+j]
	}
	return s
}

// Every command whose help says that it transmits is named among the
// commands that send telegrams: in SECURITY.md's threat model and in the
// root help. The variables of the secrets are named in SECURITY.md.
func TestTransmittingCommandsDocumented(t *testing.T) {
	root := newRoot(&app{})
	var names []string
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if strings.Contains(c.Short, "transmits") {
			names = append(names, strings.TrimPrefix(c.CommandPath(), "knxctl "))
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	if len(names) != 6 {
		t.Fatalf("transmitting commands %v", names)
	}
	security := readDoc(t, "SECURITY.md")
	threat := between(t, security, "**Passive by default.**", "- **")
	help := between(t, strings.Join(strings.Fields(root.Long), " "), "Commands that send telegrams are explicit:", "knxctl serve")
	for _, n := range names {
		if !strings.Contains(threat, "`"+n+"`") {
			t.Errorf("SECURITY.md does not name `%s` among the commands that send telegrams:\n%s", n, threat)
		}
		if !strings.Contains(help, n) {
			t.Errorf("the root help does not name %s among the commands that send telegrams:\n%s", n, help)
		}
	}
	secrets := between(t, security, "## Secrets", "## KNX Secure")
	for _, v := range append(append([]string{}, secretVars...), "KNXCTL_SECURE_USERS_FILE") {
		if !strings.Contains(secrets, "`"+v+"`") {
			t.Errorf("SECURITY.md (Secrets) does not name `%s`", v)
		}
	}
}
