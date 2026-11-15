# Security

## Reporting

Report vulnerabilities privately, not in public issues: email
**security@otfabric.io**, or open a private advisory through GitHub's
security advisories of otfabric/knxctl ("Report a vulnerability").

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | No |

## Threat model

KNX installations control lights, blinds, heating, ventilation, access
control and alarms. knxctl can send group telegrams to them, so it is a tool
that can change physical state.

- **Passive by default.** `discover`, `describe`, `diagnose`, `tunnel
  connect`, `monitor`, `group listen`, `routing monitor`, `busmonitor`,
  `dpt` and `decode` send no telegram onto the KNX network. Only `group
  write`, `group respond`, `group answer` and `group read` do, to the group
  named on the command line, and `property read` and `property write` with
  `--device` (remote device management: A_PropertyValue_Read or _Write, with
  `--connected` in a transport connection, with a tool key after S-A_Sync),
  to the device named on the command line; nothing is scanned or guessed.
  `property write` with `--device` changes that device's configuration.
  `property write` without `--device` and `tunnel feature set` send nothing
  onto the KNX network but change the configuration of the KNXnet/IP
  server. `discover` sends one search request (up to three with
  `--repeat`).
- **Validation first.** Addresses, datapoint types, values, secret files
  and numeric flags are checked before a connection is opened; an
  invocation that exits 2 or 3 sent nothing. A numeric flag value that the
  platform's `int` cannot hold (beyond 2147483647 on 32-bit builds such as
  linux/armv7) is refused with exit 2, never used as another value.
- **No confirmation prompt.** Unlike interfaces that need a privilege,
  KNXnet/IP needs none: anyone who reaches a gateway's UDP port 3671, or the
  multicast group of a routing network, can send telegrams — with knxctl or
  any other tool. knxctl does not make this worse, but it does not prevent
  it either. Protect the KNX IP network: a separate VLAN, no exposure to
  the internet or guest networks, and KNX Secure where devices support it.
- **`group answer` and `serve`** make knxctl behave like a device or a
  gateway. Run them on test installations and isolated networks: a
  simulator joined to a production routing network (`serve --routing`)
  forwards its clients' telegrams there.
- **Untrusted input.** Frames given to `decode`, telegrams received from
  the network and `serve` configuration files may be hostile. Their parsers
  live in go-knx and are fuzzed there; knxctl reads configuration files only
  where `--config` points and never executes anything from them. Strings
  from a peer or a file (a server's friendly name in `discover --format
  table` and `decode`, the creator, date, interface type and multicast
  address of a keyring in `keyring show`) are printed with control
  characters, Unicode format and bidirectional characters, invalid UTF-8 and
  backslashes escaped (`\x1b`, `\u202e`, `\\`); JSON output carries them as
  JSON strings.
- **Bounded resources.** Keyrings, `--config` files, secret files and
  `--secure-users-file` are read up to 16 MiB; a larger file exits 3.
  `group read --all` keeps at most 1000 responses. The streams hold at
  most 1024 telegrams for stdout and warn on stderr about the ones they
  dropped. So does `serve`: its printer runs apart from the server, so a
  stdout that does not take the output (a pager that stopped reading, a
  paused terminal) does not hold up the server or its clients. A telegram
  that does not fit is dropped and counted: the next record has a
  `dropped` field, and serve ends with a warning on stderr. After Ctrl-C,
  `serve` waits at most 2 s for the queued records to be written.

## Secrets

knxctl reads keyrings (`--keyring`) and secure tunnelling and routing
secrets: `--keyring-password`, `--user-password`, `--device-password`,
`--backbone-key`, `--tool-key` and the users of `serve --secure-user`.
Command lines are visible to other users of a host and stay in shell
histories, so give secrets in a file or in the environment:

| Secret | File | Environment |
|---|---|---|
| `--keyring-password` | `--keyring-password-file` | `KNXCTL_KEYRING_PASSWORD`, `KNXCTL_KEYRING_PASSWORD_FILE` |
| `--user-password` | `--user-password-file` | `KNXCTL_USER_PASSWORD`, `KNXCTL_USER_PASSWORD_FILE` |
| `--device-password` | `--device-password-file` | `KNXCTL_DEVICE_PASSWORD`, `KNXCTL_DEVICE_PASSWORD_FILE` |
| `--backbone-key` | `--backbone-key-file` | `KNXCTL_BACKBONE_KEY`, `KNXCTL_BACKBONE_KEY_FILE` |
| `--tool-key` | `--tool-key-file` | `KNXCTL_TOOL_KEY`, `KNXCTL_TOOL_KEY_FILE` |
| `serve --secure-user` | `serve --secure-users-file` | `KNXCTL_SECURE_USERS_FILE` |

A secret file holds the value; a trailing newline (LF or CRLF) is not part
of it, and an empty file is refused. A users file holds one
`id:password[:tunnel address]` per line; empty lines and lines that start
with `#` are skipped. The command line wins over the environment; a flag
and its `-file` flag, or a variable and its `_FILE` variable, exclude each
other (exit 3). The plain secret flags and passwords in `--secure-user`
still work, for tests, and are marked deprecated in the help.

knxctl never prints keys or passwords — `keyring show --secrets` is the one
deliberate exception — and error messages name users, files, lines and
addresses only (`serve --secure-user` errors mask the password, and a
malformed line of `--secure-users-file` is not shown). The secret flags
have no default value, and the environment is read after the command line
is parsed, so the usage text (`--help`) and the generated `docs/cli.md`
hold no secret, whatever the environment holds. knxctl writes nothing to
disk except where you redirect its output.

### Secret values in help output of earlier releases

From v0.13.0 the secret flags show no default in their usage text. If you
saved or shared the help output (`knxctl … --help`) of an earlier release,
or generated `docs/cli.md` with `make docs`, in a shell where
`KNXCTL_KEYRING_PASSWORD`, `KNXCTL_USER_PASSWORD`,
`KNXCTL_DEVICE_PASSWORD`, `KNXCTL_BACKBONE_KEY` or `KNXCTL_TOOL_KEY` was
set, check that output for a `(default "…")` after these flags. Where one
appears, change that password or key. `make docs` now runs the binary
with no `KNXCTL_` variable set.

## KNX Secure

knxctl uses go-knx's KNX IP Secure (secure tunnelling over TCP, secure
routing) and KNX Data Secure (group communication). With security
configured, knxctl never falls back to plain KNXnet/IP: a failed handshake
is an error (exit code 6). Not covered: general point to point Data
Secure (knxctl answers S-A_Sync and acts as a surrogate only). The implementation follows public sources and is
verified for interoperability with xknx and Calimero; it is not certified.