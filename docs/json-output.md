# JSON output

`--format json` (or `KNXCTL_FORMAT=json`) makes every command machine
readable. Every document carries `"schemaVersion": "1.0"`; the major version
changes only with an incompatible change. Fields may be added in minor
versions: ignore fields you do not know. Diagnostics (`-v`, progress lines
such as `listening via ...`) always go to stderr, never into the JSON.

## Results

Commands with one result write one indented document, the envelope:

```json
{
  "schemaVersion": "1.0",
  "command": "group read",
  "result": { ... }
}
```

`command` is the command path (`discover`, `describe`, `tunnel connect`,
`group write`, `group respond`, `group read`, `group answer`, `diagnose`,
`dpt list`, `dpt describe`, `dpt encode`, `dpt decode`, `decode`,
`version`). The `result` per command:

| Command | `result` fields |
|---------|-----------------|
| `discover` | `devices`: array of devices (below), empty when nobody answered |
| `describe` | `device`: a device |
| `tunnel connect` | `server`, `channel`, `individualAddress` (the tunnelling address), `local` (local endpoint), `connectMs`; with `--hold` also `held` (duration) and `heartbeatsOk` (the connection was alive at the end) |
| `group write`, `group respond` | `group`, `service` (`write` or `response`), `data` (hex), `small` (6-bit value in the APCI), `dpt` (with `--dpt`), `via` (`tunnel <gateway>` or `routing <group>`) |
| `group read` | `group`, `responses`: array of telegram records (below), one without `--all`, every response in the window with it |
| `group answer` | `group`, `answered` (number of reads answered) |
| `diagnose` | `server`, `ok`, `checks`: array of `name`, `ok`, `detail`, `ms` (checks: directed search, description, tunnelling service, tunnel connect, heartbeat, disconnect) |
| `dpt list` | `types`: array of datapoint types (below) |
| `dpt describe` | a datapoint type |
| `dpt encode`, `dpt decode` | `dpt`, `data` (hex), `small`, `value`, `text` (formatted with unit) |
| `decode` | `layer` (`KNXnet/IP` or `cEMI`), `service`, `length`, `fields` (header fields of the service: `channel`, `sequence`, `status`, `control`, `data`, `discovery`, `connectionType`, `layer`, `address`, `device`, `body`, `cemiError`), `cemi` (below) |
| `version` | `version`, `tag`, `commit`, `buildDate`, `goKnx`, `go`, `platform` |

A **device** (discover, describe, decoded search and description
responses): `endpoint`, `name`, `individualAddress`, `medium`, `serial`,
`mac`, `programmingMode`, `projectInstallation`, `services` (family →
version, such as `{"Core": 1, "Tunnelling": 1}`), and when present
`multicast`, `securedServices`, `extended` (answer to an extended search),
`tunnels` (array of `address`, `free`, `status`), `knxAddresses`, `ip`
(`configuredIP`, `currentIP`, ...), `otherBlocks` (types of other
description blocks).

A **datapoint type**: `id`, `name`, `description`, `unit`, `bits`, `kind`
(`bool`, `float`, `enum`, ...), `min`/`max` (when the type has a range),
`labels` (DPT 1), `values` (enumerations, `"1=comfort"`).

A decoded **cEMI** frame: `messageCode`, `source`, `destination`,
`priority`, `hopCount`, `standardFrame`, `confirmError`, `tpci`, `apci`,
`data` (hex), `raw` (the whole frame in hex).

**Values.** Numbers, booleans and strings decode to JSON numbers, booleans
and strings; enumerations to `{"value": 1, "name": "comfort"}`; times,
dates and colours to their text form (`"Mon 12:30:00"`, `"#FF8000"`).

## Streams

`monitor`, `group listen`, `routing monitor` and `serve` write JSON Lines:
one compact object per line, with `schemaVersion` and `type` first. Every
record has `"type": "telegram"`:

| Field | |
|-------|-|
| `time` | RFC 3339 time with nanoseconds, UTC |
| `source` | individual address of the sender |
| `group` | destination group address |
| `service` | `read`, `write` or `response` |
| `data` | the value octets in hex (not for `read`) |
| `small` | `true` when the value is a 6-bit value in the APCI octet (DPT 1, 2, 3) |
| `dpt` | the datapoint type it was decoded with (`--map`, `--dpt`; `serve`: the object's type) |
| `value`, `text` | the decoded value and its formatted text |
| `decodeError` | the value does not fit the datapoint type |

```json
{"schemaVersion":"1.0","type":"telegram","time":"2026-10-29T10:00:00.123456789Z","source":"1.1.1","group":"1/2/4","service":"response","data":"0C33","dpt":"9.001","value":21.5,"text":"21.5 °C"}
```

The same records are the elements of `responses` in `group read`.

## Errors

A fatal error is one document on stdout, and the exit code tells its kind
([exitcodes.md](exitcodes.md)):

```json
{
  "schemaVersion": "1.0",
  "error": {"kind": "timeout", "code": "timeout", "message": "knx: timeout: client: no response to the read of 1/2/4"}
}
```

Do not parse `message`; use `kind` and `code`. `diagnose` with failing
checks writes its result and exits 1 without an error document.
