# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

## v0.1.0

**Date:** 2026-10-29
**Previous release:** none (first release)

## Summary

knxctl, a command-line tool for KNX over KNXnet/IP: discover and inspect
gateways, monitor group telegrams, read, write and answer group objects,
encode and decode datapoint types and frames, and run a simulated KNXnet/IP
server. Built on go-knx v0.1.0.

## Additions

- **Discovery and inspection:** `discover` (multicast search,
  `--extended`, `--unicast` directed search, `--repeat`), `describe`,
  `diagnose` (directed search, description, services, tunnel connect,
  heartbeat, disconnect; exit 1 when a check fails), `tunnel connect`
  (`--hold`, `--heartbeat`, `--tunnel-address`, `--nat`).
- **Monitoring (passive):** `monitor`, `group listen [group...]` and
  `routing monitor`, through a tunnel (`--gateway`, `KNXCTL_GATEWAY`) or
  routing (`--routing`, `--multicast`), decoding values with `--map
  group=dpt` and `--dpt`; `--count`, `--duration`; JSON Lines with
  `--format json`.
- **Group communication (transmits):** `group write` and `group respond`
  (DPT values with `--dpt`, booleans without, `--raw` octets, `--small`
  6-bit values), `group read` (`--all` window; exit 5 without a response),
  `group answer` (answers reads like a device with the R flag).
- **Offline:** `dpt list`, `dpt describe`, `dpt encode`, `dpt decode`;
  `decode` of KNXnet/IP frames and cEMI frames (`--cemi`).
- **Simulator:** `knxctl serve` with group objects from `--object
  group[+group]:dpt[:flags][=value]` or a JSON configuration (`--config`,
  docs/serve.md); tunnelling, discovery, optional routing; every telegram
  printed.
- **Output:** text, table and versioned JSON (`schemaVersion` 1.0), a JSON
  error document on failure, stable exit codes (docs/exitcodes.md); a hint
  for negative values read as flags (`--`).

## Verification

- Tests drive `cmd.Run` in process against real go-knx servers on
  127.0.0.1 and against `knxctl serve`; routing tests over multicast
  loopback (required in CI with `KNX_REQUIRE_MULTICAST=1`).
- Whole-module coverage gated at 85 % (`make coverage`).

---
