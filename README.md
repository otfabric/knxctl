# knxctl

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI Status](https://github.com/otfabric/knxctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/knxctl/actions/workflows/ci.yml)
[![Code Coverage](https://codecov.io/gh/otfabric/knxctl/graph/badge.svg)](https://codecov.io/gh/otfabric/knxctl)
[![Latest Release](https://img.shields.io/github/v/release/otfabric/knxctl?label=release)](https://github.com/otfabric/knxctl/releases)

A command-line tool for KNX over KNXnet/IP: discover gateways, inspect
them, monitor group telegrams, read and switch group objects, encode and
decode datapoint types and frames, and run a simulated KNXnet/IP server.
Built on [go-knx](https://github.com/otfabric/go-knx); pure Go, one static
binary.

```sh
knxctl discover                                              # who is on the network
knxctl monitor -g 192.168.1.10 --map 1/2/4=9.001             # every group telegram, passively
knxctl group read 1/2/4 --dpt 9.001 -g 192.168.1.10          # read a temperature
knxctl group write 1/2/3 on -g 192.168.1.10                  # switch a light
knxctl routing monitor --interface eth0                      # KNX IP routing multicast
knxctl serve --object 1/2/3:1.001=off --object 1/2/4:9.001:CRT=21.5   # a simulated gateway
knxctl dpt encode 9.001 21.5                                 # 0C33
knxctl decode 061004200015040100001100bce0110a0a03010081     # a captured frame
```

## Passive by default

knxctl sends nothing onto the KNX network unless the command says so:

- `discover`, `describe`, `diagnose` and `tunnel connect` talk only to the
  KNXnet/IP server (`discover` sends one search request, no scan); no
  telegram reaches the KNX bus.
- `monitor`, `group listen`, `routing monitor` and `busmonitor` only listen.
- `property read` reads a server's interface object properties (local
  device management); nothing reaches the KNX bus. With `--device` it reads
  a device on the KNX network (remote device management): those requests do
  reach the bus.
- `dpt` and `decode` work offline.
- Only `group write`, `group respond`, `group answer`, `group read` and
  `property read`/`write --device` send telegrams (a read is a telegram that devices answer). They validate the
  address, datapoint type and value before connecting: an invocation that
  exits 2 or 3 has sent nothing.

Writing to group addresses switches real loads — lights, blinds, heating,
access control. Try commands against `knxctl serve` first.

## Install

Binaries for Linux (amd64, arm64, armv7), macOS (amd64, arm64) and Windows
(amd64, arm64) are attached to each
[GitHub release](https://github.com/otfabric/knxctl/releases). From source:

```sh
go install github.com/otfabric/knxctl@latest
```

Shell completion: `knxctl completion bash|zsh|fish|powershell`, for example
`knxctl completion zsh > "${fpath[1]}/_knxctl"`.

## Commands

| Command | | Sends telegrams? |
|---------|-|:-:|
| `discover` | Find KNXnet/IP servers (multicast search, or `--unicast` directed search; `--programming-mode`, `--mac`, `--service` filters) | no |
| `describe host` | Read a server's self description | no |
| `tunnel connect host` | Open a tunnelling connection, report it, disconnect | no |
| `diagnose host` | Check search, description, services, tunnelling, heartbeat | no |
| `monitor` | Print every group telegram (tunnel or routing) | no |
| `group listen [group...]` | Print the telegrams of some or all groups | no |
| `routing monitor` | Print the routing indications on the multicast group | no |
| `busmonitor host` | Print every bus frame through a busmonitor connection | no |
| `property read host object pid` | Read an interface object property (device management; `--device` for a device on the KNX network, secured with its tool key when known) | no (`--device`: a request to that device) |
| `property write host object pid hex` | Write an interface object property (device management; `--device` as for read) | configures the server (`--device`: the device) |
| `tunnel feature get/set host feature` | Tunnelling v2 interface features (max APDU, bus connection, info service...) | no |
| `group write group value` | Send A_GroupValue_Write | **yes** |
| `group respond group value` | Send A_GroupValue_Response | **yes** |
| `group read group` | Send A_GroupValue_Read and print the response | **yes** |
| `group answer group value` | Answer reads of a group, like a device with the R flag | **yes** |
| `dpt list / describe / encode / decode` | Datapoint types, offline | no |
| `decode hex` | Decode a KNXnet/IP or cEMI frame, offline | no |
| `keyring show file` | Verify and summarise an ETS keyring (secrets only with `--secrets`) | no |
| `serve` | Run a simulated KNXnet/IP server | its own virtual network |
| `version`, `completion` | | |

Every command that tunnels takes `--tcp` to use KNXnet/IP over TCP (core v2)
instead of UDP, where the server supports it.

Full reference with every flag: [docs/cli.md](docs/cli.md).

### Discovering gateways

```sh
knxctl discover                              # multicast search on 224.0.23.12:3671, 5 s window
knxctl discover --interface eth0 --timeout 2s
knxctl discover --extended --format json     # SEARCH_REQUEST_EXTENDED (core v2)
knxctl discover --unicast 192.168.1.10       # directed search of one server
knxctl describe 192.168.1.10                 # services, tunnelling slots, addresses
knxctl diagnose 192.168.1.10                 # exits 1 when a check fails
knxctl tunnel connect 192.168.1.10 --hold 1m --heartbeat 30s
```

### Monitoring group telegrams

```sh
knxctl monitor -g 192.168.1.10
knxctl monitor -g 192.168.1.10 --map 1/2/4=9.001 --map 1/2/3=1.001
knxctl group listen 1/2/3 1/2/4 --map 1/2/4=9.001 -g 192.168.1.10
knxctl monitor -g 192.168.1.10 --count 100 --format json > telegrams.jsonl
knxctl monitor -g 192.168.1.10 --duration 10m --dpt 9.001   # decode every group as 9.001
```

`--map group=dpt` decodes a group with a datapoint type, `--dpt` decodes
all others; telegrams that do not fit are shown as undecodable. Streams end
at `--count`, `--duration` or Ctrl-C with exit 0.

### Reading temperatures

```sh
knxctl group read 1/2/4 --dpt 9.001 -g 192.168.1.10
# 2026-10-29T10:00:00.12Z 1.1.1     -> 1/2/4     response 0C33 (21.5 °C)
knxctl group read 1/2/4 --dpt 9.001 -g 192.168.1.10 --format json
knxctl group read 1/2/4 --all 2s -g 192.168.1.10     # every device that answers
```

A response is not guaranteed: the read exits 5 when no device with the R
flag answers within `--timeout`.

### Switching group objects

```sh
knxctl group write 1/2/3 on -g 192.168.1.10                 # DPT 1 without --dpt: on/off, true/false, 1/0
knxctl group write 1/2/5 50 --dpt 5.001 -g 192.168.1.10     # 50 % brightness
knxctl group write 1/2/6 21.5 --dpt 9.001 -g 192.168.1.10   # a setpoint
knxctl group write 1/2/7 comfort --dpt 20.102 -g 192.168.1.10
knxctl group write 1/2/8 --raw 0C33 -g 192.168.1.10         # octets as given
knxctl group write 1/2/9 --raw 09 --small -g 192.168.1.10   # a 6-bit value in the APCI (DPT 3)
knxctl group respond 1/2/4 21.5 --dpt 9.001 -g 192.168.1.10
knxctl group answer 1/2/4 21.5 --dpt 9.001 -g 192.168.1.10 --count 1
```

**Negative values.** A value that starts with `-` is read as a flag. Put
`--` before the arguments:

```sh
knxctl group write --dpt 9.001 -g 192.168.1.10 -- 1/2/6 -5.5
knxctl dpt encode 9.001 -- -30
```

Without `--` knxctl exits 2 and prints this hint.

### Monitoring routing traffic

```sh
knxctl routing monitor --interface eth0
knxctl routing monitor --multicast 239.255.1.1:3671 --map 1/2/4=9.001
knxctl monitor --routing --count 10 --format json
knxctl group write 1/2/3 on --routing --source 1.1.250 --interface eth0
```

Routing joins the KNX IP multicast group (224.0.23.12:3671 by default) and
sends routing indications with the source address `--source` (set it to an
address of your line: without it the source is 0.0.0). Multicast loopback is
on, so knxctl processes and simulators on the same host hear each other.

### Connecting to simulated environments with knxctl serve

```sh
# Terminal 1: a gateway with a switch and a temperature sensor
knxctl serve --listen 127.0.0.1:3671 --no-discovery \
  --object 1/2/3:1.001:CRWU=off --object 1/2/4:9.001:CRT=21.5 --tunnels 1.1.10,1.1.11

# Terminal 2
knxctl group read 1/2/4 --dpt 9.001 -g 127.0.0.1
knxctl group write 1/2/3 on -g 127.0.0.1
knxctl monitor -g 127.0.0.1 --map 1/2/3=1.001
```

Larger setups come from a JSON file (`--config site.json`): devices, group
objects with flags and initial values, tunnelling addresses. The simulator
also serves ETS, Home Assistant, xknx and other KNXnet/IP clients. See
[docs/serve.md](docs/serve.md) for the schema and what is (not) emulated.

### Busmonitor and device management

```bash
knxctl busmonitor 192.168.1.10 --count 20       # raw TP1 frames, decoded
knxctl property read 192.168.1.10 device 11     # serial number
knxctl property read 192.168.1.10 knxnetip 76 --count 15 --tcp   # friendly name
knxctl property read 192.168.1.10 device 11 --device 1.1.5        # serial number of device 1.1.5
knxctl property read 192.168.1.10 device 11 --device 1.1.5 --connected --keyring site.knxkeys --keyring-password ...
```

With `--device`, the request goes through a tunnel to that device on the KNX
network (A_PropertyValue_Read/_Write, connectionless or with `--connected` in
a transport connection); the object type is looked up in the device's
interface object list. When the device has a tool key (`--tool-key`, or its
entry in `--keyring`), the access is secured with KNX Data Secure (S-A_Sync,
then S-A_Data with tool access) and never falls back to plain. `knxctl serve
--tool-key HEX|keyring` makes the simulator's own device accept only such
secured access.

A busmonitor connection is exclusive on a KNX interface: it cannot be opened
while a tunnel is connected, and blocks tunnels while it is open. `knxctl serve
--concurrent-busmonitor` lifts this in the simulator.

### KNX Secure

knxctl speaks KNX IP Secure (secure tunnelling over TCP, secure routing) and
KNX Data Secure (secured group communication), with the keys of an ETS
keyring or given directly:

```bash
export KNXCTL_KEYRING=project.knxkeys KNXCTL_KEYRING_PASSWORD=...   # secrets from the environment
knxctl keyring show project.knxkeys                     # users, groups, devices (no secrets)
knxctl group read 1/2/4 --gateway 192.168.1.10          # secure tunnel as the keyring's first user, Data Secure groups decrypted
knxctl group write 1/2/3 on --gateway 192.168.1.10 --user-id 3
knxctl monitor --routing                                # secure routing with the keyring's backbone key
KNXCTL_USER_PASSWORD=... knxctl tunnel connect 192.168.1.10 --user-id 2 --device-password ...
knxctl serve --keyring project.knxkeys --keyring-host 1.1.0   # a secure simulator: the host's users
knxctl serve --secure-user 2:secret:1.1.10 --device-password trustme
knxctl group write 1/2/3 on --gateway 192.168.1.10 --user-id 2 --udp   # the secure session over UDP
knxctl serve --routing --keyring project.knxkeys --keyring-host 1.1.0  # secure routing with the keyring's backbone key
```

A keyring selects the tunnelling user by `--tunnel-address`, `--user-id`
or as the first tunnelling user; user 1 is the host's management password
(device management). Secured telegrams are marked `[secure]` (`"secure":
true` in JSON). Data Secure covers group communication; with a keyring
device entry (tool key) for its own address, knxctl answers S-A_Sync and
acts as the surrogate through which tools such as Calimero send secured
group telegrams. The implementation follows public sources and is
verified against xknx and Calimero, not certified (go-knx's
[SPECIFICATIONS](https://github.com/otfabric/go-knx/blob/main/docs/standards/SPECIFICATIONS.md#knx-secure-sources)).

### Datapoint types and frames

```sh
knxctl dpt list --main 9 --format table
knxctl dpt describe 20.102
knxctl dpt encode 10.001 'Mon 12:30:00'
knxctl dpt decode 9.001 0C33
knxctl decode 061004200015040100001100bce0110a0a03010081
knxctl decode --cemi 2900bce0110a0a03010081 --format json
```

## Global flags

| Flag | Default | |
|------|---------|-|
| `--format` | `text` | `text`, `table` or `json` (env `KNXCTL_FORMAT`) |
| `--timeout` | `5s` | Each request: search window, connect, read |
| `-i, --interface` | | Network interface for multicast and the announced address |
| `--local-addr` | | Local address to bind (IP or IP:port) |
| `-v`, `-vv` | | Protocol events on stderr (info, debug) |
| `-q, --quiet` | | Nothing on stderr except the error |

Link flags of the commands that reach the KNX network: `-g, --gateway
host[:port]` (env `KNXCTL_GATEWAY`; port 3671 by default), `--routing`,
`--multicast`, `--nat`, `--source`, `--tunnel-address`.

## Configuration

knxctl has no configuration file. Two environment variables set defaults:

| Variable | |
|----------|-|
| `KNXCTL_FORMAT` | Default `--format` (`json` for scripts); `KNXCTL_OUTPUT_FORMAT` is accepted too, `KNXCTL_FORMAT` wins |
| `KNXCTL_GATEWAY` | Default `--gateway`, so that `knxctl group read 1/2/4` needs no flag |
| `KNXCTL_KEYRING`, `KNXCTL_KEYRING_PASSWORD` | Default `--keyring` and its password |
| `KNXCTL_USER_PASSWORD`, `KNXCTL_DEVICE_PASSWORD`, `KNXCTL_BACKBONE_KEY` | Secure tunnelling and routing secrets without a keyring |

Flags given on the command line win.

## Output

Results go to stdout, diagnostics and progress to stderr. `--format text`
(default) is for people, `--format table` aligns columns for list-like
results (`discover`, `dpt list`, `diagnose`), and `--format json` writes one
versioned document per result, JSON Lines for streams, and a JSON error
document on failure: [docs/json-output.md](docs/json-output.md). Exit codes
are stable: [docs/exitcodes.md](docs/exitcodes.md).

## Troubleshooting

- **`discover` finds nothing.** Multicast must reach the gateway and back.
  Choose the interface (`--interface eth0`); a host with several interfaces
  may send on the wrong one. Docker's default bridge network, most VPNs,
  Wi-Fi with client isolation and cloud networks do not carry multicast:
  use `discover --unicast host` or `describe host`, or run the container
  with `--network host`.
- **`routing monitor` is silent.** As above; also check that the routers
  use the same multicast group (`--multicast`) and that a firewall admits
  UDP 3671. Routing does not cross routers without multicast routing.
- **Tunnelling through NAT** (Docker port mappings, a gateway behind a
  router): add `--nat`. knxctl then announces the route back endpoint
  0.0.0.0:0 and the server answers to where the request came from; the
  server must support route back endpoints (`knxctl serve` does).
- **Connect is refused (exit 6).** Every tunnelling slot is in use: close
  other clients (ETS, visualisations) or wait for stale connections to time
  out (120 s). `describe` lists the slots on servers that report them.
- **`group read` times out (exit 5).** No device with the R flag on that
  group answered. That is normal for many actuators' switch objects; read
  the status object instead.
- **A negative value is taken for a flag** (exit 2): put `--` before the
  arguments, `knxctl group write --dpt 9.001 -g host -- 1/2/4 -30`.
- `-v` or `-vv` logs the KNXnet/IP exchange to stderr.

## Security

knxctl never prints keys or passwords (`keyring show --secrets` excepted)
and reads them from the environment by default; avoid the flags on shared
hosts, where command lines are visible to other users. See
[SECURITY.md](SECURITY.md).

## Documentation

[docs/cli.md](docs/cli.md) · [docs/json-output.md](docs/json-output.md) ·
[docs/exitcodes.md](docs/exitcodes.md) · [docs/serve.md](docs/serve.md) ·
[SECURITY.md](SECURITY.md) · [CHANGELOG.md](CHANGELOG.md) ·
[CONTRIBUTING.md](CONTRIBUTING.md) · [RELEASE.md](RELEASE.md)

## Trademark

KNX is a registered trademark of KNX Association cvba. knxctl is an
independent implementation, not affiliated with or endorsed by KNX
Association.

## License

MIT, see [LICENSE](LICENSE).
