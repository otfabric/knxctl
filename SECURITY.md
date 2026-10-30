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

knxctl reads keyrings (`--keyring`) and secure tunnelling and routing
secrets. They come from the environment by default (`KNXCTL_KEYRING_PASSWORD`,
`KNXCTL_USER_PASSWORD`, `KNXCTL_DEVICE_PASSWORD`, `KNXCTL_BACKBONE_KEY`);
the equivalent flags exist for tests and scripts, but command lines are
visible to other users of a host. knxctl never prints keys or passwords —
`keyring show --secrets` is the one deliberate exception — and error
messages name users and addresses only (`serve --secure-user` errors mask
the password). It writes nothing to disk except where you redirect its
output.

## KNX Secure

knxctl uses go-knx's KNX IP Secure (secure tunnelling over TCP, secure
routing) and KNX Data Secure (group communication). With security
configured, knxctl never falls back to plain KNXnet/IP: a failed handshake
is an error (exit code 6). Not covered: general point to point Data
Secure (knxctl answers S-A_Sync and acts as a surrogate only). The implementation follows public sources and is
verified for interoperability with xknx and Calimero; it is not certified.