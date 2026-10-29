# knxctl command reference

Generated from `knxctl <command> --help` by scripts/gen-cli-doc.sh (`make docs`); do not edit.
Exit codes: [exitcodes.md](exitcodes.md). JSON output: [json-output.md](json-output.md).
Simulator configuration: [serve.md](serve.md).

```text
knxctl is a command-line tool for KNX over KNXnet/IP, built on
github.com/otfabric/go-knx.

Passive by default: discover, describe, monitor, group listen, routing monitor,
diagnose and the dpt and decode commands never send telegrams onto the KNX
network (discover sends one search request; describe and diagnose talk only to
the KNXnet/IP server). Commands that send telegrams are explicit: group write,
group respond, group answer and group read (a read is a telegram the devices
answer).

A KNXnet/IP server is reached by tunnelling (--gateway host[:port], env
KNXCTL_GATEWAY) or the KNX IP network by routing (--routing, multicast
224.0.23.12:3671 by default). --nat makes tunnelling work through NAT (Docker
port mappings) when the server supports route back endpoints.

knxctl serve runs a simulated KNXnet/IP server with virtual group objects.

Results go to stdout as text (default), a table or JSON (--format); diagnostics
go to stderr. Exit codes: docs/exitcodes.md.

Usage:
  knxctl [command]

Available Commands:
  completion  Generate a shell completion script
  decode      Decode a KNXnet/IP frame or a cEMI frame given in hexadecimal
  describe    Read the self description of a KNXnet/IP server
  diagnose    Check a KNXnet/IP server: description, services, tunnelling, heartbeat
  discover    Find KNXnet/IP servers with a search request
  dpt         Datapoint types: list, describe, encode, decode
  group       Group communication: read, write, respond, answer, listen
  help        Help about any command
  monitor     Print every group telegram (passive)
  routing     KNXnet/IP routing (IP multicast)
  serve       Run a simulated KNXnet/IP server with virtual group objects
  tunnel      KNXnet/IP tunnelling connections
  version     Print the version of knxctl and go-knx

Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -h, --help                help for knxctl
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)

Use "knxctl [command] --help" for more information about a command.
```

## knxctl discover

```text
Send one search request to the KNXnet/IP multicast group 224.0.23.12:3671 (or,
with --unicast, to one server) and list the servers that answer within
--timeout. It is not a scan: one request (--repeat up to 3), no probing.

Multicast must reach the servers: on Docker, VPN or cloud networks it often
does not; use --unicast or describe instead.

Usage:
  knxctl discover [flags]

Examples:
  knxctl discover
  knxctl discover --interface eth0 --timeout 2s
  knxctl discover --extended --format json
  knxctl discover --unicast 192.168.1.10

Flags:
      --extended         Send SEARCH_REQUEST_EXTENDED (KNXnet/IP core v2)
  -h, --help             help for discover
      --nat              Announce the route back endpoint (directed search through NAT)
      --repeat int       Send the search 1..3 times, one second apart (default 1)
      --unicast string   Directed search of one server, host[:port]

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl describe

```text
Read the self description of a KNXnet/IP server

Usage:
  knxctl describe host[:port] [flags]

Examples:
  knxctl describe 192.168.1.10
  knxctl describe knx-gw.local --format json

Flags:
  -h, --help   help for describe
      --nat    Announce the route back endpoint (through NAT)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl tunnel

```text
KNXnet/IP tunnelling connections

Usage:
  knxctl tunnel [command]

Available Commands:
  connect     Open a tunnelling connection, report it and disconnect

Flags:
  -h, --help   help for tunnel

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)

Use "knxctl tunnel [command] --help" for more information about a command.
```

## knxctl tunnel connect

```text
Open a tunnelling connection, print the channel and tunnelling address, keep it
for --hold (sending heartbeats every --heartbeat) and disconnect. No telegram
is sent onto the KNX network.

Usage:
  knxctl tunnel connect host[:port] [flags]

Examples:
  knxctl tunnel connect 192.168.1.10
  knxctl tunnel connect 192.168.1.10 --hold 2m --heartbeat 30s

Flags:
      --heartbeat duration      Heartbeat interval while held (default 1m0s)
  -h, --help                    help for connect
      --hold duration           Keep the connection this long before disconnecting
      --nat                     Announce the route back endpoint (through NAT)
      --tunnel-address string   Request this tunnelling address (tunnelling v2 servers)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl group

```text
Group communication: read, write, respond, answer, listen

Usage:
  knxctl group [command]

Available Commands:
  answer      Answer A_GroupValue_Read of a group with a value, like a device with the R flag (transmits)
  listen      Print the group telegrams to some or all groups (passive)
  read        Send A_GroupValue_Read and print the response (transmits a telegram)
  respond     Send A_GroupValue_Response (transmits a telegram)
  write       Send A_GroupValue_Write (transmits a telegram)

Flags:
  -h, --help   help for group

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)

Use "knxctl group [command] --help" for more information about a command.
```

## knxctl group write

```text
Send A_GroupValue_Write (transmits a telegram).

The value is parsed with --dpt (see knxctl dpt list); without it a boolean
(on/off, true/false, 1/0) is sent as DPT 1. --raw sends octets as given.

Usage:
  knxctl group write <group> [value] [flags]

Examples:
  knxctl group write 1/2/3 on --gateway 192.168.1.10
  knxctl group write 1/2/4 21.5 --dpt 9.001 --gateway 192.168.1.10
  knxctl group write 1/2/5 --raw 0C33 --routing --source 1.1.250

Flags:
  -t, --dpt string              Datapoint type of the value (9.001)
  -g, --gateway string          KNXnet/IP server to tunnel through, host[:port] (env KNXCTL_GATEWAY)
  -h, --help                    help for write
      --multicast string        Routing multicast group (default "224.0.23.12:3671")
      --nat                     Tunnel through NAT: announce the route back endpoint 0.0.0.0:0
      --raw string              Send these octets (hexadecimal) instead of a value
      --routing                 Use KNXnet/IP routing (IP multicast) instead of a tunnel
      --small                   With --raw: send one octet 00..3F in the APCI (6-bit value)
      --source string           Source individual address of sent telegrams (routing; a tunnel uses the server's)
      --tunnel-address string   Request this tunnelling address (extended CRI, tunnelling v2 servers)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl group respond

```text
Send A_GroupValue_Response (transmits a telegram).

The value is parsed with --dpt (see knxctl dpt list); without it a boolean
(on/off, true/false, 1/0) is sent as DPT 1. --raw sends octets as given.

Usage:
  knxctl group respond <group> [value] [flags]

Examples:
  knxctl group respond 1/2/4 21.5 --dpt 9.001 --gateway 192.168.1.10

Flags:
  -t, --dpt string              Datapoint type of the value (9.001)
  -g, --gateway string          KNXnet/IP server to tunnel through, host[:port] (env KNXCTL_GATEWAY)
  -h, --help                    help for respond
      --multicast string        Routing multicast group (default "224.0.23.12:3671")
      --nat                     Tunnel through NAT: announce the route back endpoint 0.0.0.0:0
      --raw string              Send these octets (hexadecimal) instead of a value
      --routing                 Use KNXnet/IP routing (IP multicast) instead of a tunnel
      --small                   With --raw: send one octet 00..3F in the APCI (6-bit value)
      --source string           Source individual address of sent telegrams (routing; a tunnel uses the server's)
      --tunnel-address string   Request this tunnelling address (extended CRI, tunnelling v2 servers)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl group read

```text
Send A_GroupValue_Read and print the first A_GroupValue_Response within
--timeout. A response is not guaranteed: the read times out (exit 5) when no
device answers. Several devices may answer the same address; --all collects
every response during the given window.

Usage:
  knxctl group read <group> [flags]

Examples:
  knxctl group read 1/2/4 --dpt 9.001 --gateway 192.168.1.10
  knxctl group read 1/2/4 --all 2s --gateway 192.168.1.10 --format json

Flags:
      --all duration            Collect every response during this window
  -t, --dpt string              Datapoint type to decode the value with (9.001)
  -g, --gateway string          KNXnet/IP server to tunnel through, host[:port] (env KNXCTL_GATEWAY)
  -h, --help                    help for read
      --multicast string        Routing multicast group (default "224.0.23.12:3671")
      --nat                     Tunnel through NAT: announce the route back endpoint 0.0.0.0:0
      --routing                 Use KNXnet/IP routing (IP multicast) instead of a tunnel
      --source string           Source individual address of sent telegrams (routing; a tunnel uses the server's)
      --tunnel-address string   Request this tunnelling address (extended CRI, tunnelling v2 servers)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl group answer

```text
Wait for A_GroupValue_Read telegrams to the group and answer each with
A_GroupValue_Response of the value, until --count answers, --duration or
interruption. This makes knxctl behave as a device with the R flag on that
group: use it on test installations and simulators.

Usage:
  knxctl group answer <group> [value] [flags]

Examples:
  knxctl group answer 1/2/4 21.5 --dpt 9.001 --gateway 192.168.1.10 --count 1

Flags:
  -n, --count int               Stop after this many answers
  -t, --dpt string              Datapoint type of the value
      --duration duration       Stop after this time
  -g, --gateway string          KNXnet/IP server to tunnel through, host[:port] (env KNXCTL_GATEWAY)
  -h, --help                    help for answer
      --multicast string        Routing multicast group (default "224.0.23.12:3671")
      --nat                     Tunnel through NAT: announce the route back endpoint 0.0.0.0:0
      --raw string              Answer with these octets (hexadecimal)
      --routing                 Use KNXnet/IP routing (IP multicast) instead of a tunnel
      --small                   With --raw: one octet 00..3F in the APCI
      --source string           Source individual address of sent telegrams (routing; a tunnel uses the server's)
      --tunnel-address string   Request this tunnelling address (extended CRI, tunnelling v2 servers)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl group listen

```text
Print the group telegrams to some or all groups (passive)

Usage:
  knxctl group listen [group...] [flags]

Examples:
  knxctl group listen 1/2/3 1/2/4 --map 1/2/4=9.001 --gateway 192.168.1.10
  knxctl group listen --routing --count 10 --format json

Flags:
  -n, --count int               Stop after this many telegrams
  -t, --dpt string              Decode every group with this datapoint type
      --duration duration       Stop after this time
  -g, --gateway string          KNXnet/IP server to tunnel through, host[:port] (env KNXCTL_GATEWAY)
  -h, --help                    help for listen
      --map stringArray         Decode a group with a datapoint type: group=dpt (repeatable)
      --multicast string        Routing multicast group (default "224.0.23.12:3671")
      --nat                     Tunnel through NAT: announce the route back endpoint 0.0.0.0:0
      --routing                 Use KNXnet/IP routing (IP multicast) instead of a tunnel
      --source string           Source individual address of sent telegrams (routing; a tunnel uses the server's)
      --tunnel-address string   Request this tunnelling address (extended CRI, tunnelling v2 servers)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl monitor

```text
Print every group telegram seen through a tunnel (--gateway) or on the routing
multicast group (--routing). Nothing is sent onto the KNX network.

Usage:
  knxctl monitor [flags]

Examples:
  knxctl monitor --gateway 192.168.1.10 --map 1/2/4=9.001
  knxctl monitor --routing --interface eth0

Flags:
  -n, --count int               Stop after this many telegrams
  -t, --dpt string              Decode every group with this datapoint type
      --duration duration       Stop after this time
  -g, --gateway string          KNXnet/IP server to tunnel through, host[:port] (env KNXCTL_GATEWAY)
  -h, --help                    help for monitor
      --map stringArray         Decode a group with a datapoint type: group=dpt (repeatable)
      --multicast string        Routing multicast group (default "224.0.23.12:3671")
      --nat                     Tunnel through NAT: announce the route back endpoint 0.0.0.0:0
      --routing                 Use KNXnet/IP routing (IP multicast) instead of a tunnel
      --source string           Source individual address of sent telegrams (routing; a tunnel uses the server's)
      --tunnel-address string   Request this tunnelling address (extended CRI, tunnelling v2 servers)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl routing

```text
KNXnet/IP routing (IP multicast)

Usage:
  knxctl routing [command]

Available Commands:
  monitor     Print the routing indications on the multicast group (passive)

Flags:
  -h, --help   help for routing

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)

Use "knxctl routing [command] --help" for more information about a command.
```

## knxctl routing monitor

```text
Print the routing indications on the multicast group (passive)

Usage:
  knxctl routing monitor [flags]

Examples:
  knxctl routing monitor --interface eth0 --map 1/2/4=9.001

Flags:
  -n, --count int           Stop after this many telegrams
  -t, --dpt string          Decode every group with this datapoint type
      --duration duration   Stop after this time
  -h, --help                help for monitor
      --map stringArray     Decode a group with a datapoint type: group=dpt (repeatable)
      --multicast string    Routing multicast group (default "224.0.23.12:3671")

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl diagnose

```text
Run a sequence of checks against a KNXnet/IP server: directed search,
description request, supported service families, tunnelling connection,
connection state (heartbeat) and disconnection. No telegram is sent onto the KNX
network. Exits 1 when a check fails.

Usage:
  knxctl diagnose host[:port] [flags]

Examples:
  knxctl diagnose 192.168.1.10
  knxctl diagnose 192.168.1.10 --format json

Flags:
  -h, --help   help for diagnose
      --nat    Announce route back endpoints (through NAT)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl dpt

```text
Datapoint types: list, describe, encode, decode

Usage:
  knxctl dpt [command]

Available Commands:
  decode      Decode the octets of a group telegram
  describe    Describe a datapoint type
  encode      Encode a value to the octets of a group telegram
  list        List the datapoint types

Flags:
  -h, --help   help for dpt

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)

Use "knxctl dpt [command] --help" for more information about a command.
```

## knxctl dpt list

```text
List the datapoint types

Usage:
  knxctl dpt list [flags]

Examples:
  knxctl dpt list
  knxctl dpt list --main 9

Flags:
  -h, --help       help for list
      --main int   Only the subtypes of this main number

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl dpt describe

```text
Describe a datapoint type

Usage:
  knxctl dpt describe <dpt> [flags]

Examples:
  knxctl dpt describe 9.001

Flags:
  -h, --help   help for describe

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl dpt encode

```text
Encode a value to the octets of a group telegram

Usage:
  knxctl dpt encode <dpt> <value> [flags]

Examples:
  knxctl dpt encode 9.001 21.5
  knxctl dpt encode 10.001 'Mon 12:30:00'
  knxctl dpt encode 232.600 '#FF8000'

Flags:
  -h, --help   help for encode

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl dpt decode

```text
Decode the octets of a group telegram

Usage:
  knxctl dpt decode <dpt> <hex> [flags]

Examples:
  knxctl dpt decode 9.001 0C33
  knxctl dpt decode 1.001 01

Flags:
  -h, --help   help for decode

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl decode

```text
Decode a KNXnet/IP frame or a cEMI frame given in hexadecimal

Usage:
  knxctl decode <hex> [flags]

Examples:
  knxctl decode 061004200015040100001100bce0110a0a03010081
  knxctl decode --cemi 2900bce0110a0a03010081

Flags:
      --cemi   The octets are a cEMI frame, not a KNXnet/IP frame
  -h, --help   help for decode

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl serve

```text
Run a KNXnet/IP server (discovery, description, tunnelling, optionally
routing) with a virtual KNX network behind it: telegrams of one tunnelling
client reach the other clients and the virtual group objects, which store
written values and answer reads according to their flags. Every telegram is
printed. This is a simulator for development and tests; it does not emulate a
physical KNX line or device management.

Group objects come from --object group[+group]:dpt[:flags][=value] on a
device at --device-address, or from a JSON file (--config, docs/serve.md).

Usage:
  knxctl serve [flags]

Examples:
  knxctl serve --object 1/2/3:1.001:CRWU=off --object 1/2/4:9.001:CRT=21.5
  knxctl serve --config site.json --routing --interface eth0
  knxctl serve --listen 127.0.0.1:3671 --no-discovery --tunnels 1.1.10,1.1.11

Flags:
      --address string          Individual address of the server (default 15.15.0)
      --config string           JSON configuration file (docs/serve.md)
      --device-address string   Individual address of the --object device (default "1.1.1")
  -h, --help                    help for serve
      --listen string           UDP address to listen on (default ":3671")
      --name string             Friendly name (default "knxctl serve")
      --no-discovery            Do not join the multicast group for search requests
      --object stringArray      Group object group[+group]:dpt[:flags][=value] (repeatable; flags default CRWU)
      --routing                 Route telegrams over IP multicast too
      --tunnels strings         Tunnelling addresses, one per connection (default 15.15.250)

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl version

```text
Print the version of knxctl and go-knx

Usage:
  knxctl version [flags]

Flags:
  -h, --help   help for version

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```

## knxctl completion

```text
Generate a shell completion script

Usage:
  knxctl completion bash|zsh|fish|powershell [flags]

Flags:
  -h, --help   help for completion

Global Flags:
      --format string       Output format: text, json or table (env KNXCTL_FORMAT) (default "text")
  -i, --interface string    Network interface for multicast and the announced address
      --local-addr string   Local address to bind (IP or IP:port)
  -q, --quiet               Log nothing to stderr
      --timeout duration    Timeout of each request (search window, connect, read) (default 5s)
  -v, --verbose count       Log protocol events to stderr (-v info, -vv debug)
```
