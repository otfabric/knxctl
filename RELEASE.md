# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

## v0.6.0

**Date:** 2026-10-30
**Previous release:** v0.5.1

## Added

- `property read` and `property write --device IA`: remote device
  management of a device on the KNX network through a tunnel
  (A_PropertyValue_Read/_Write; the object type is found in the device's
  interface object list), connectionless or with `--connected` in a
  transport connection.
- KNX Data Secure tool access: with the device's tool key (`--tool-key`,
  env `KNXCTL_TOOL_KEY`, or the device's entry in `--keyring`) the access is
  secured (S-A_Sync, then S-A_Data with tool access); results report
  `secured`.
- `serve --tool-key HEX|keyring`: the simulator's own device accepts only
  remote management secured with that tool key. The simulator's device now
  answers remote device management, and its friendly name is writable.

## Changes

- go-knx v0.5.0.

---

## v0.5.1

**Date:** 2026-10-30
**Previous release:** v0.5.0

## Fixes

- A command interrupted before it had a result (Ctrl-C while connecting,
  for example) exits 130 with kind `interrupted`, as the sibling CLIs do;
  it was reported as an internal error (exit 10).
- `tunnel connect --hold` says on stderr when it is connected and
  holding.
- Tests: the interrupt and server-gone tests of `tunnel connect` waited
  for the server to register the connection, which happens before the
  client has its CONNECT_RESPONSE; under load the client was interrupted
  while still connecting (the flaky failure). They now wait for the
  client's own notice.

---

## v0.5.0

**Date:** 2026-10-30
**Previous release:** v0.4.0

## Summary

On go-knx v0.4.1, the go-knx API features that were missing from the CLI.

## Additions

- `property write host object pid hex`: M_PropWrite (`--start`, `--count`,
  `--instance`).
- `tunnel feature get|set host feature`: the tunnelling v2 feature
  services, by name (max-apdu, bus-connection, address, info-service...)
  or number; TCP by default, `--udp` for UDP.
- `discover --programming-mode`, `--mac`, `--service family[:version]`:
  extended search with search parameters.
- `routing monitor --keyring/--backbone-key`: secure routing and Data
  Secure, like `monitor --routing`.
- `diagnose --tcp` and the secure session flags; a `security` check lists
  the secured services and tells when tunnelling needs a secure session.
- `--priority` on the commands that send telegrams.
- `serve --programming-mode`, `--serial`, `--multicast`.

## Fixes

- go-knx v0.4.1: a server on an ephemeral port keeps TCP.

---

## v0.4.0

**Date:** 2026-10-30
**Previous release:** v0.3.0

## Summary

On go-knx v0.4.0: secure sessions over UDP, secure routing in `serve`,
and S-A_Sync with the surrogate role.

## Additions

- `--udp` on every tunnelling command: the KNX IP Secure session over UDP
  instead of TCP.
- `serve --routing` routes securely with the keyring's backbone key or
  `--backbone-key`.
- With a keyring that has a tool key for knxctl's own address, knxctl
  answers S-A_Sync and group object diagnostics: Calimero tools send their
  Data Secure group telegrams through it.

---

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
