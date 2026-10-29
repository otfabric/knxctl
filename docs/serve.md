# knxctl serve: a simulated KNXnet/IP server

`knxctl serve` runs a KNXnet/IP server (go-knx `server` package) with a
virtual KNX network behind it. KNXnet/IP clients — knxctl itself, ETS, Home
Assistant's KNX integration, xknx, Calimero, knxd's `ipt:` driver — connect
to it as to a KNX IP interface and exchange group telegrams with each
other and with virtual group objects. Every telegram on the virtual network
is printed to stdout (text lines or JSON Lines, see
[json-output.md](json-output.md)).

```sh
knxctl serve --object 1/2/3:1.001:CRWU=off --object 1/2/4:9.001:CRT=21.5
knxctl serve --config site.json --listen 127.0.0.1:3671 --no-discovery
knxctl serve --config site.json --routing --interface eth0
```

## Flags

| Flag | Default | |
|------|---------|-|
| `--listen` | `:3671` | UDP address of the control and data endpoint |
| `--name` | `knxctl serve` | Friendly name in descriptions (at most 30 characters); overrides the configuration's `name` when given |
| `--address` | `15.15.0` | Individual address of the server |
| `--tunnels` | `15.15.250` | Tunnelling addresses, comma separated; one per connection, so their number is the connection limit |
| `--object` | | A group object (repeatable, below) |
| `--device-address` | `1.1.1` | Individual address of the device holding the `--object` objects |
| `--config` | | JSON configuration file (below) |
| `--routing` | off | Also route telegrams over IP multicast (224.0.23.12:3671); needs `--listen` on port 3671 of every address (`:3671`) |
| `--no-discovery` | off | Do not join the multicast group for search requests; directed (unicast) searches and descriptions are still answered |
| `--interface` (global) | | Interface whose address is announced and on which the multicast group is joined |

The server listens until Ctrl-C (SIGINT/SIGTERM), then disconnects its
clients and exits 0. The bound address is reported on stderr (`serving on
127.0.0.1:3671 (2 devices; Ctrl-C to stop)`).

## --object

```text
--object group[+group...]:dpt[:flags][=value]
```

- `group`: one or more group addresses joined with `+`; the first is the
  sending address (read responses are sent to it). The object is named
  after this part.
- `dpt`: the datapoint type (`knxctl dpt list`), such as `1.001`, `9.001`.
- `flags`: communication flags, default `CRWU`:
  - `C` communication: the object takes part at all;
  - `R` read: answers `A_GroupValue_Read` with its value;
  - `W` write: takes over the value of `A_GroupValue_Write`;
  - `U` update: takes over the value of `A_GroupValue_Response`;
  - `T` transmit and `I` read on init are accepted but have no effect in the
    simulator (no application changes values on its own).
- `=value`: the initial value in the datapoint type's text form (`on`,
  `21.5`, `comfort`, `'Mon 12:30:00'`). Without one the object has no value
  and does not answer reads until it is written.

All `--object` objects belong to one device named `virtual` at
`--device-address`; they are added after the devices of `--config`.

## Configuration file

```json
{
  "name": "test site",
  "address": "1.0.0",
  "tunnels": ["1.1.30", "1.1.31"],
  "devices": [
    {
      "name": "living room",
      "address": "1.1.2",
      "objects": [
        {"name": "light", "dpt": "1.001", "flags": "CRWU", "groups": ["2/0/1", "2/0/2"], "value": "on"},
        {"name": "temperature", "dpt": "9.001", "flags": "CRT", "groups": ["2/1/1"], "value": "21.5"},
        {"name": "hvac mode", "dpt": "20.102", "groups": ["2/2/1"], "value": "comfort"}
      ]
    }
  ]
}
```

| Field | Type | |
|-------|------|-|
| `name` | string | Friendly name (`--name` overrides it) |
| `address` | string | Individual address of the server (`--address` when absent) |
| `tunnels` | array of strings | Tunnelling addresses (`--tunnels` when absent) |
| `devices` | array | Virtual devices |
| `devices[].name` | string | Name, for messages |
| `devices[].address` | string | Individual address: the source of its read responses |
| `devices[].objects` | array | Group objects |
| `objects[].name` | string | Name, for messages |
| `objects[].dpt` | string | Datapoint type |
| `objects[].flags` | string | Flag letters; `CRWU` when absent or empty |
| `objects[].groups` | array of strings | Group addresses, the first being the sending address |
| `objects[].value` | string | Initial value in text form (optional) |

Unknown fields are ignored. An invalid file, address, datapoint type, flag
or value exits 3 before anything is opened.

## What is simulated

- KNXnet/IP core services: search (multicast and directed), extended
  search, description, connect, connection state (heartbeat), disconnect;
  connections without heartbeat for 120 s are closed.
- Tunnelling on the link layer over UDP (tunnelling v1), including the
  extended CRI that requests a tunnelling address, and route back (NAT)
  endpoints. Each `L_Data.req` of a client is confirmed (`L_Data.con`) and
  delivered as `L_Data.ind` to the other clients, the virtual devices and,
  with `--routing`, the multicast group.
- Routing indications (`--routing`), rate limited to 50 per second.
- Group objects that store written values and answer reads according to
  their flags, at most one response per read per device.

## What is not simulated

- A physical KNX line: no bus timing, collisions, line couplers, filter
  tables, repetitions or bus load; every telegram arrives.
- Device management: programming mode, individual address assignment,
  memory, properties, restart, ETS download.
- Busmonitor and raw tunnelling, device management connections,
  KNXnet/IP over TCP, tunnelling feature services.
- KNX Secure (KNXnet/IP Secure and KNX Data Secure).
- Applications: no object changes its value on its own, so `T` never fires.

Use it for development, tests, demonstrations and training — not as a
replacement for a KNX IP interface in a building.
