# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

## v0.10.0

**Date:** 2026-11-07
**Previous release:** v0.9.0

## Summary

`knxctl version` prints the one-line format shared by every OT Fabric CLI, and `version --json` (or `--format json`) the matching JSON object. Shell completion now covers the values of every flag and argument with a fixed set, in bash, zsh, fish and PowerShell.

**Upgrade notes:** the output of `version` changed (see Changed). Its JSON is no longer wrapped in the `schemaVersion`/`command`/`result` envelope; scripts that read `version --format json` must read the new keys (`goKnx` is now an entry of `libraries`, `buildDate` is `built`). No other command, exit code or JSON document changed.

## Changes

### Added

- `version --json`: one line, `{"name","version","tag","commit","built","libraries","go","platform"}` in that order, `libraries` the OT Fabric libraries as `[{"name":"go-knx","version":"0.8.1"}]`, every key present (`"dev"`/`"unknown"` when unknown, `"tag": ""` without a tag). `--format json` and `KNXCTL_FORMAT=json` print the same.
- Shell completion of values: `--format`, `--priority` (low, normal, urgent, system), `--service` (service families), `--dpt`/`-t` (every datapoint type, with its name), the datapoint of `dpt describe`/`encode`/`decode`, the feature of `tunnel feature get`/`set`, the object of `property read`/`write`. `--keyring` and `keyring show` complete `.knxkeys` files and `serve --config` `.json` files; every other flag and argument (addresses, numbers, hex) completes no file names.

### Changed

- `version` prints one line, `knxctl <version> (tag <tag>, commit <commit>, built <date>, go-knx <version>, <go>, <os>/<arch>)`, without the leading `v` and with the full commit; the tag part is left out without a tag. A development build (`go build`, `go run`, `make build` off a tag) reports `dev` and the commit with `-dirty` from the toolchain's VCS stamp; `go install …@vX.Y.Z` reports X.Y.Z. It was three lines: `knxctl v0.9.0 (commit)`, the go-knx version, the Go version and platform.
- `make build` injects the version and tag only when HEAD is exactly on a tag (without the leading `v`; otherwise the binary reports `dev`) and the full commit.

### Documentation

- docs/json-output.md: the `version` object. README: what shell completion completes.

---

## v0.9.0

**Date:** 2026-11-07
**Previous release:** v0.8.0

## Summary

Built on go-knx v0.8.1 (was v0.7.0): routing flow control, TCP connections kept over an unparsable frame, KNX IP Secure session timers and 30 more datapoint types. `serve` gets `--secure-session-timeout`, and docs/serve.md now describes what the simulator does.

**Upgrade notes:** a secure `knxctl serve` now ends KNX IP Secure sessions idle for 60 s (was 120 s); set `--secure-session-timeout` to change it. Routing senders (`group write --routing`, `serve --routing`) are paced and pause after a ROUTING_BUSY, so a burst of telegrams takes longer. Exit codes and JSON output are unchanged.

## Changes

### Added

- `serve --secure-session-timeout` (default 60 s): how long a KNX IP Secure session may stay idle. It needs a secure server (`--secure-user` or `--keyring`), and zero or a negative value exits 3.
- `dpt list`, `encode`, `decode` and `serve --object` know 30 more datapoint types from go-knx v0.8.0 (239, was 209), among them 5.100, 15.000, 20.107, 21.x, 22.x, 27.001, 217.001, 222.100/101, 234.001/002, 275.100/101.

### Changed

- go-knx v0.8.1, through v0.8.0. The routing client and the server's routing honour ROUTING_BUSY (pause for the wait time, then a reduced rate), and the server queues and paces its routing indications instead of dropping those over the rate. A TCP connection (secure tunnel or `serve`) is kept when one frame's body does not parse, the tunnel client answers a DISCONNECT_REQUEST for another channel with E_CONNECTION_ID and keeps the tunnel, and it sends its secure session keep alive after 30 s idle (was 45 s). A secure server ends sessions idle for 60 s. See go-knx's release notes.

### Documentation

- docs/serve.md: the flag table lists every flag. "What is simulated" now covers TCP, busmonitor, device management of the server's device, feature services, KNX IP Secure and routing flow control; until now it listed them as not simulated.
- README: routing sends follow flow control.

---

## v0.8.0

**Date:** 2026-11-05
**Previous release:** v0.7.2

## Summary

Built on go-knx v0.7.0 and Go 1.26. Static analysis (gosec, opengrep) at low/low is clean, with every finding fixed in code. A hardening pass fixed 15 defects where flags were not checked, a peer could hang or flood a command, or the exit code or JSON output was wrong. New chaos, signal and fuzz tests cover them.

**Upgrade notes:** Go 1.26 to build. Several invalid inputs that were accepted now exit 3 (see Changed).

## Changes

### Fixed

- `property --device`: a device listing more than 256 interface objects made an object at index 256 or later come back at a wrapped index (256 as 0), so the command addressed another object. Objects past index 255 are no longer found.
- Local `property read`/`write`: a server that acknowledged the request and never confirmed it hung the command forever. `tunnel feature get`/`set` waited 3 s whatever `--timeout` said. Both now end at `--timeout` (exit 5).
- `group read --all` kept every response, so a flood (routing multicast is open to the LAN) grew memory without limit. The read now ends at the 1000th response, with a warning on stderr (go-knx v0.7.0 `WithMaxResponses`).
- A very large `--timeout` wrapped to a negative deadline, and commands exited 5 at once. Deadlines now saturate.
- Ctrl-C during a remote property access exited 5 (timeout) instead of 130.
- `--format json` streams (monitor, busmonitor, routing monitor) that failed after their records appended an indented error document. stdout is now valid JSON Lines: the error is one compact line.
- `--count -1` on `busmonitor` and `group answer` exited 0 at once. On the monitors, a negative `--count` meant no limit and a negative `--duration` meant forever. A negative `group read --all` shortened the deadline, and a negative `tunnel connect --hold` was ignored. All of these now exit 3.
- `--raw` over 254 octets, property data over 1007 octets (248 with `--device`) and a feature value over 1012 octets were refused only after connecting. A local property write then tore the tunnel down (exit 4). They are now refused before connecting (exit 3, `invalid_value`).
- `--keyring` and `serve --config` read the whole file (`/dev/zero` or a FIFO grew memory or hung). Files over 16 MiB are refused.
- `diagnose` checked credentials only after the network checks. It now checks them first (exit 3).
- `dpt list --main` outside 0..65535 returned an empty list. It now exits 3.
- Hex input accepts a `0X` prefix like `0x`.

### Changed

- go-knx v0.7.0, through v0.6.0. v0.6.0 fixed the server reconnect sequence counters, exact-endpoint channel frames and Windows UDP read errors. v0.7.0 bounds search and group read results and device management waits, keeps a cancelled access apart from a timeout, and refuses oversized frames without dropping the tunnel. See their notes. Go 1.26 (CI on 1.26 and 1.27), `go fix` applied, shared workflows v3.3.1, pflag v1.0.10.
- Interrupted commands whose cause go-knx reported as a timeout exit 130 with code `canceled`, and the message starts with "interrupted: ".
- Tests: `cmd/chaos_test.go` (noise, flood and silent peers against every network command), `e2e/signal_test.go` (SIGINT to the binary), and fuzz targets for busmonitor records, the object list, raw values, search filters and redaction.

---

## v0.7.2

**Date:** 2026-10-31
**Previous release:** v0.7.1

### Changed

- Policy 1.2: build output in `bin/` and `release/`, a per-package coverage floor, and more tests and CI. No change in behaviour.
- Built on go-knx v0.5.2.

---

## v0.7.1

**Date:** 2026-10-31
**Previous release:** v0.7.0

### Fixed

- go-knx v0.5.1: `knxctl` commands that start a local server (and the tests) no longer fail on Windows with "no port free for UDP and TCP" (see its release notes).

---

## v0.7.0

**Date:** 2026-10-30
**Previous release:** v0.6.0

Exit codes converge on the organisation table (ADR-0006). **Scripts that test for these codes must change.**

| Case | Old | New |
|---|---|---|
| Operating system refused an operation (`permission`) | 7 | 9 |
| KNXnet/IP server refused the request (`rejected`) | 9 | 6 |

All other codes are unchanged. The JSON `kind` values do not change: `rejected` stays `rejected`. Exit 7 is now reserved for a partial result, which knxctl does not produce today.

`KNXCTL_OUTPUT_FORMAT` is now accepted besides `KNXCTL_FORMAT`, which is the canonical name and wins when both are set.

---

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
