// SPDX-License-Identifier: MIT

// Command knxctl discovers, inspects, monitors and talks to KNX
// installations over KNXnet/IP, encodes and decodes datapoint types, and
// simulates KNXnet/IP servers.
package main

import (
	"os"

	"github.com/otfabric/knxctl/cmd"
)

// Injected at link time: -X main.version=... etc.
var (
	version   = "dev"
	tag       = ""
	commit    = ""
	buildDate = ""
)

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	os.Exit(cmd.Execute())
}
