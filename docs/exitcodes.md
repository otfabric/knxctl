# Process exit codes

knxctl's exit codes are stable for scripting (they are defined in
`internal/errs`). A fatal error is rendered once: as one `error:` line on
stderr, or, with `--format json`, as a single JSON error document on stdout
(see [json-output.md](json-output.md)). `--format` (and `KNXCTL_FORMAT`) is
honoured also for usage errors. `KNXCTL_OUTPUT_FORMAT` is accepted too;
`KNXCTL_FORMAT` is the canonical name and wins when both are set.

The table follows the organisation table
(otfabric/engineering `standards/api-design.md#cli-exit-codes`). Older versions used 7 for
`permission` and 9 for `rejected`; see RELEASE.md.

| Code | Kind | Meaning |
|------|------|---------|
| 0 | | Success. Streams (`monitor`, `group listen`, `routing monitor`, `group answer`, `serve`) also end with 0 at `--count`, `--duration` or Ctrl-C (SIGINT/SIGTERM) |
| 1 | | Findings: a result was written and it reports problems — `diagnose` had a failing check. No error document follows |
| 2 | `usage` | Unknown command or flag, missing argument, a value a flag cannot hold, no `--gateway`/`--routing` for a command that needs the KNX network, neither a value nor `--raw` for a write. A negative value read as a flag (`-30`) also exits 2: put `--` before the arguments |
| 3 | `invalid_input` | A group or individual address, datapoint type, value, hex string, `--map` entry, `--object` specification, configuration file or flag combination that does not make sense. Nothing was sent |
| 4 | `transport` | The network failed: a socket could not be opened or bound (port in use), the tunnelling connection was lost or closed by the server |
| 5 | `timeout` | A deadline passed: the KNXnet/IP server did not answer a connect or description request, or no device answered `group read` (`code` is `timeout`; with `--all` and no response in the window `no_response`) |
| 6 | `protocol`, `rejected` | A malformed frame (`decode`, or from the server), a telegram the server did not confirm (`not_confirmed`), a KNX Secure failure. `rejected`: the KNXnet/IP server refused the request: no free tunnelling slot, an unknown tunnelling address, busy (`code` `rejected` or `busy`) |
| 8 | `unsupported` | The server or go-knx does not support what was asked |
| 9 | `permission` | The operating system refused an operation (for example binding a privileged port) |
| 10 | `output`, `internal` | The result could not be written, or something unexpected happened |
| 130 | `interrupted` | Ctrl-C (SIGINT) or SIGTERM stopped a command before it had a result, for example while connecting (`code` `canceled`). Streams stopped by Ctrl-C end with 0 |

`kind` selects the exit code; `code` in the JSON error is a stable, finer
identifier:

| `code` | Kind |
|--------|------|
| `invalid_input` | `usage`, `invalid_input` |
| `invalid_address` | `invalid_input` |
| `invalid_value` | `invalid_input` |
| `network_failed` | `transport` |
| `connection_lost` | `transport` |
| `timeout` | `timeout` |
| `no_response` | `timeout` |
| `malformed_frame` | `protocol` |
| `not_confirmed` | `protocol` |
| `security_failed` | `protocol` |
| `permission_denied` | `permission` |
| `unsupported` | `unsupported` |
| `rejected` | `rejected` |
| `busy` | `rejected` |
| `output_failed` | `output` |
| `canceled` | `interrupted` |
| `internal_unexpected` | `internal` |

**Transmitting commands** (`group write`, `group respond`, `group read`,
`group answer`) parse and validate the address, datapoint type and value
before connecting: an invocation that exits 2 or 3 has sent nothing.

**Streams** that lose their link (the server disconnects the tunnel) end
with exit 4 and an error document.
