# Security

## Reporting

Report vulnerabilities privately through GitHub's security advisories of
otfabric/knxctl ("Report a vulnerability"), not in public issues.

## Threat model

KNX installations control lights, blinds, heating, ventilation, access
control and alarms. knxctl can send group telegrams to them, so it is a tool
that can change physical state.

- **Passive by default.** `discover`, `describe`, `diagnose`, `tunnel
  connect`, `monitor`, `group listen`, `routing monitor`, `dpt` and `decode`
  send no telegram onto the KNX network. Only `group write`, `group
  respond`, `group answer` and `group read` do, and only to the group named
  on the command line; nothing is scanned or guessed. `discover` sends one
  search request (up to three with `--repeat`).
- **Validation first.** Addresses, datapoint types and values are checked
  before a connection is opened; an invocation that exits 2 or 3 sent
  nothing.
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
  where `--config` points and never executes anything from them.

## Secrets

knxctl handles no secrets yet: it has no credentials, keyrings or
passwords, and writes nothing to disk except where you redirect its output.

## KNX Secure

KNX Secure (KNXnet/IP Secure tunnelling and routing, KNX Data Secure) is
not implemented. knxctl talks plain KNXnet/IP only: telegrams are neither
encrypted nor authenticated, and a secured gateway or secured group address
cannot be used. When KNX Secure support arrives, keys will be read from a
keyring file named on the command line, never from arguments or the
environment, and never printed.
