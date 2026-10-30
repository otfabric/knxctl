# AGENTS.md — knxctl

## What this is

Command-line tool for KNX over KNXnet/IP built on [go-knx](https://github.com/otfabric/go-knx): discover gateways, monitor telegrams, read and write group objects, encode and decode datapoints and frames, run a simulated KNXnet/IP server. Pure Go, one static binary. Profile: **cli**.

## Organisation standards

This repository follows **otfabric/engineering policy 1.0**, profile **cli** (`.otfabric/policy.yaml`, mode `baseline`). Where this file and the organisation standards disagree, this file wins here — say so in your report.

## Where the truth lives

| Question | Source |
|---|---|
| Exit codes (script-facing API) | docs/exitcodes.md |
| JSON output contract | docs/json-output.md |
| Command reference | docs/cli.md (generated: `make docs`) |
| `serve` | docs/serve.md |
| KNX behaviour | go-knx |
| Release history | RELEASE.md (`## vX.Y.Z` … `---`) |

## Hard rules

1. Exit codes and JSON output are a public contract: changing a code or a field is a breaking change with a RELEASE.md note.
2. Transmitting commands validate before connecting: an invocation that exits 2 or 3 has sent nothing.
3. Never lower `COVERAGE_MIN`, add lint suppressions, or skip or delete tests to make CI pass.
4. `docs/cli.md` is regenerated, never hand-edited.
5. go-knx is pinned to a released tag; no `replace` directives.

## Commands

| When you change… | Run |
|---|---|
| anything | `make check` |
| the command line, exit codes or `main` | `make e2e` (builds the binary and runs it) |
| a command's flags or help | `make docs` |
| routing or multicast paths | `make test-multicast` |

## Definition of Done

`make check` green; tests for new commands; docs/cli.md regenerated; exit codes and JSON documented; RELEASE.md section for user-visible changes.

## Git and releases

Branches and PRs. Do not push to `main`, tag or run the Release workflow unless the maintainer asks in the session.
