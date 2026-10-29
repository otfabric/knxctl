# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

## v0.3.0

**Date:** 2026-10-30
**Previous release:** v0.2.0

## Summary

KNX IP Secure and KNX Data Secure, on go-knx v0.3.0.

## Additions

- `--keyring` (ETS .knxkeys) on every command that reaches the network:
  secure tunnelling as a keyring user (`--tunnel-address`, `--user-id`, or
  the first user; user 1 is the host's management password), Data Secure
  for the keyring's groups, secure routing with its backbone key.
- `--user-id`, `--user-password`, `--device-password`, `--backbone-key`
  without a keyring. Secrets default to environment variables.
- `serve --keyring/--keyring-host` and `serve --secure-user
  id:password[:address] --device-password`: a secure simulator; Data
  Secure telegrams are printed decrypted when the keyring has the key.
- `keyring show`: verify and summarise a keyring (`--secrets` to show keys
  and passwords).
- Secured telegrams are marked `[secure]` / `"secure": true`.
- docs/cli.md now covers `busmonitor`, `property read` and `keyring`.

---

## v0.2.0

**Date:** 2026-10-29
**Previous release:** v0.1.1

## Summary

KNXnet/IP over TCP, busmonitor and device management, on go-knx v0.2.0.

## Additions

- `--tcp` on every tunnelling command (`group`, `monitor`, `tunnel
  connect`, `busmonitor`, `property read`): KNXnet/IP over TCP.
- `busmonitor host`: every bus frame through a busmonitor connection
  (L_Busmon.ind), decoded from the TP1 frame; text or JSON Lines.
- `property read host object pid`: interface object properties with local
  device management (`--start`, `--count`, `--instance`); objects by type
  number or `device`, `router`, `knxnetip`.
- `serve --no-tcp` and `serve --concurrent-busmonitor`.

---

## v0.1.1

**Date:** 2026-10-29
**Previous release:** v0.1.0

## Fixes

- `knxctl version` reports the module version of builds made with
  `go install github.com/otfabric/knxctl@vX.Y.Z` (it said "dev": such
  builds carry no link-time version).

---

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
