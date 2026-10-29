# knxctl

[![CI](https://github.com/otfabric/knxctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/knxctl/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/otfabric/knxctl/branch/main/graph/badge.svg)](https://codecov.io/gh/otfabric/knxctl)

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
- `monitor`, `group listen` and `routing monitor` only listen.
- `dpt` and `decode` work offline.
- Only `group write`, `group respond`, `group answer` and `group read` send
  telegrams (a read is a telegram that devices answer). They validate the
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
| `discover` | Find KNXnet/IP servers (multicast search, or `--unicast` directed search) | no |
| `describe host` | Read a server's self description | no |
| `tunnel connect host` | Open a tunnelling connection, report it, disconnect | no |
| `diagnose host` | Check search, description, services, tunnelling, heartbeat | no |
| `monitor` | Print every group telegram (tunnel or routing) | no |
| `group listen [group...]` | Print the telegrams of some or all groups | no |
| `routing monitor` | Print the routing indications on the multicast group | no |
| `group write group value` | Send A_GroupValue_Write | **yes** |
| `group respond group value` | Send A_GroupValue_Response | **yes** |
| `group read group` | Send A_GroupValue_Read and print the response | **yes** |
| `group answer group value` | Answer reads of a group, like a device with the R flag | **yes** |
| `dpt list / describe / encode / decode` | Datapoint types, offline | no |
| `decode hex` | Decode a KNXnet/IP or cEMI frame, offline | no |
| `serve` | Run a simulated KNXnet/IP server | its own virtual network |
| `version`, `completion` | | |

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
| `KNXCTL_FORMAT` | Default `--format` (`json` for scripts) |
| `KNXCTL_GATEWAY` | Default `--gateway`, so that `knxctl group read 1/2/4` needs no flag |

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
- **Connect is refused (exit 9).** Every tunnelling slot is in use: close
  other clients (ETS, visualisations) or wait for stale connections to time
  out (120 s). `describe` lists the slots on servers that report them.
- **`group read` times out (exit 5).** No device with the R flag on that
  group answered. That is normal for many actuators' switch objects; read
  the status object instead.
- **A negative value is taken for a flag** (exit 2): put `--` before the
  arguments, `knxctl group write --dpt 9.001 -g host -- 1/2/4 -30`.
- `-v` or `-vv` logs the KNXnet/IP exchange to stderr.

## Security

knxctl handles no secrets yet; KNX Secure is not implemented. See
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
