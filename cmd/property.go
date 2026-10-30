// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/otfabric/go-knx/devmgmt"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

var objectNames = map[string]uint16{
	"device": devmgmt.ObjectDevice, "router": devmgmt.ObjectRouter, "knxnetip": devmgmt.ObjectKNXnetIPParameter,
}

func parseObjectType(s string) (uint16, error) {
	if v, ok := objectNames[strings.ToLower(s)]; ok {
		return v, nil
	}
	v, err := strconv.ParseUint(s, 0, 16)
	if err != nil {
		return 0, errs.Invalid("object %q: a type number (0x000B) or device, router, knxnetip", s)
	}
	return uint16(v), nil
}

type propertyResult struct {
	Server string `json:"server"`
	// Device, ObjectIndex and Secured are set for remote device
	// management (--device).
	Device      string `json:"device,omitempty"`
	ObjectIndex *uint8 `json:"objectIndex,omitempty"`
	Secured     bool   `json:"secured,omitempty"`
	ObjectType  uint16 `json:"objectType"`
	Instance    uint8  `json:"instance"`
	PropertyID  uint8  `json:"propertyId"`
	Start       uint16 `json:"start"`
	Count       uint8  `json:"count"`
	Data        string `json:"data"`
	Printable   string `json:"text,omitempty"`
}

func (r propertyResult) Text() string {
	s := fmt.Sprintf("object 0x%04X/%d property %d [%d..%d]: %s", r.ObjectType, r.Instance, r.PropertyID, r.Start, int(r.Start)+int(r.Count)-1, r.Data)
	if r.Device != "" {
		s = fmt.Sprintf("%s object 0x%04X (index %d) property %d [%d..%d]: %s", r.Device, r.ObjectType, *r.ObjectIndex, r.PropertyID, r.Start, int(r.Start)+int(r.Count)-1, r.Data)
		if r.Secured {
			s += " [tool access]"
		}
	}
	if r.Printable != "" {
		s += fmt.Sprintf(" (%q)", r.Printable)
	}
	return s
}

func newPropertyCmd(a *app) *cobra.Command {
	var sec secureFlags
	cmd := &cobra.Command{Use: "property", Short: "Interface object properties of a KNXnet/IP server or of a device on the KNX network"}
	var instance, count uint8
	var start uint16
	var tcp, nat bool
	var rf remoteFlags
	read := &cobra.Command{
		Use:   "read host[:port] <object> <pid>",
		Short: "Read a property (local M_PropRead, or A_PropertyValue_Read of a device with --device)",
		Long: `Open a device management connection and read count elements of a property
from element start (start 0 reads the number of elements). The object is a type
number or device (0x0000), router (0x0006) or knxnetip (0x000B, the KNXnet/IP
parameter object). Nothing is sent onto the KNX network.

With --device the property is read from that device on the KNX network instead,
through a tunnel (remote device management): the object type is looked up in the
device's interface object list. A device with a tool key (--tool-key, or in
--keyring) is accessed with KNX Data Secure (S-A_Sync, then S-A_Data with tool
access); --connected uses a transport connection.`,
		Example: `  knxctl property read 192.168.1.10 device 11          # serial number
  knxctl property read 192.168.1.10 knxnetip 76 --count 30   # friendly name
  knxctl property read 192.168.1.10 knxnetip 53 --start 0     # number of tunnelling addresses
  knxctl property read 192.168.1.10 device 11 --device 1.1.5  # serial number of device 1.1.5
  knxctl property read 192.168.1.10 device 11 --device 1.1.5 --keyring p.knxkeys --keyring-password ...   # secured with its tool key`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			obj, err := parseObjectType(args[1])
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[2], 0, 8)
			if err != nil {
				return errs.Invalid("property id %q: 0..255", args[2])
			}
			if count < 1 || count > 15 || start > 0x0FFF {
				return errs.Invalid("--count is 1..15, --start 0..4095")
			}
			if rf.device != "" {
				return remoteRead(cmd, a, args[0], obj, uint8(pid), start, count, instance, tcp, nat, &sec, &rf)
			}
			if rf.connected || rf.toolKey != "" {
				return errs.Invalid("--connected and --tool-key need --device")
			}
			c, err := dialDevMgmt(cmd, a, args[0], tcp, nat, &sec)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()
			b, err := c.PropRead(cmd.Context(), obj, instance, uint8(pid), start, count)
			if err != nil {
				return err
			}
			r := propertyResult{Server: args[0], ObjectType: obj, Instance: instance, PropertyID: uint8(pid), Start: start, Count: count, Data: strings.ToUpper(hex.EncodeToString(b))}
			r.Printable = printable(b)
			return a.emit("property read", r)
		},
	}
	read.Flags().Uint8Var(&instance, "instance", 1, "Object instance")
	read.Flags().Uint16Var(&start, "start", 1, "First element (0: the number of elements)")
	read.Flags().Uint8Var(&count, "count", 1, "Number of elements (1..15)")
	read.Flags().BoolVar(&tcp, "tcp", false, "Connect over TCP")
	sec.add(read.Flags())
	read.Flags().BoolVar(&nat, "nat", false, "Announce the route back endpoint (through NAT)")
	rf.add(read.Flags())
	cmd.AddCommand(read, newPropertyWriteCmd(a))
	return cmd
}

// printable returns the octets as text when they are printable ASCII
// (trailing NULs dropped), else "".
func printable(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	for _, c := range b {
		if c != 0 && (c < 0x20 || c > 0x7E) {
			return ""
		}
	}
	return strings.TrimRight(string(b), "\x00")
}

// remoteRead reads a property of a device on the KNX network (--device).
func remoteRead(cmd *cobra.Command, a *app, host string, obj uint16, pid uint8, start uint16, count, instance uint8, tcp, nat bool, sec *secureFlags, rf *remoteFlags) error {
	if instance != 1 {
		return errs.Invalid("--instance: remote device management addresses objects by index (instance 1)")
	}
	ctx, cancel := remoteContext(cmd.Context(), a)
	defer cancel()
	r, err := dialRemote(ctx, a, host, tcp, nat, sec, rf)
	if err != nil {
		return err
	}
	defer r.close()
	idx, err := r.objectIndex(ctx, obj)
	if err != nil {
		return err
	}
	b, err := r.d.PropertyRead(ctx, idx, pid, start, count)
	if err != nil {
		return err
	}
	res := propertyResult{Server: host, Device: r.addr.String(), ObjectIndex: &idx, Secured: r.secured, ObjectType: obj, Instance: 1,
		PropertyID: pid, Start: start, Count: count, Data: strings.ToUpper(hex.EncodeToString(b)), Printable: printable(b)}
	return a.emit("property read", res)
}

// dialDevMgmt opens a device management connection.
func dialDevMgmt(cmd *cobra.Command, a *app, host string, tcp, nat bool, sec *secureFlags) (*devmgmt.Conn, error) {
	opts := []tunnel.Option{tunnel.WithLogger(a.logger), tunnel.WithConnectTimeout(a.timeout), tunnel.WithNAT(nat)}
	if tcp {
		opts = append(opts, tunnel.WithTCP())
	}
	opts, err := sec.tunnelOptions(opts, "")
	if err != nil {
		return nil, err
	}
	return devmgmt.Dial(cmd.Context(), host, opts...)
}

func newPropertyWriteCmd(a *app) *cobra.Command {
	var sec secureFlags
	var instance, count uint8
	var start uint16
	var tcp, nat bool
	var rf remoteFlags
	write := &cobra.Command{
		Use:   "write host[:port] <object> <pid> <hex data>",
		Short: "Write a property (local M_PropWrite, or A_PropertyValue_Write of a device with --device)",
		Long: `Open a device management connection and write count elements of a property
from element start; the data holds the count elements, hexadecimal. The server
answers with the written elements, or with none when it refuses (read only,
wrong type or length). This changes the server's configuration.

With --device the property of that device on the KNX network is written
instead, through a tunnel (remote device management), and the device's answer
(the elements after the write) is printed; --connected and the tool key work
as for property read. This changes the device's configuration.`,
		Example: `  knxctl property write 192.168.1.10 device 54 01 --user-id 1   # programming mode on (PID_PROG_MODE)
  knxctl property write 192.168.1.10 knxnetip 76 6B6E7863746C --count 6
  knxctl property write 192.168.1.10 knxnetip 76 6B6E78 --count 3 --device 1.1.0 --connected`,
		Args: cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			obj, err := parseObjectType(args[1])
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[2], 0, 8)
			if err != nil {
				return errs.Invalid("property id %q: 0..255", args[2])
			}
			data, err := hex.DecodeString(strings.ReplaceAll(args[3], " ", ""))
			if err != nil || len(data) == 0 {
				return errs.Invalid("data %q: hexadecimal octets", args[3])
			}
			if count < 1 || count > 15 || start < 1 || start > 0x0FFF {
				return errs.Invalid("--count is 1..15, --start 1..4095")
			}
			if len(data)%int(count) != 0 {
				return errs.Invalid("%d octets do not divide into %d elements", len(data), count)
			}
			if rf.device != "" {
				if instance != 1 {
					return errs.Invalid("--instance: remote device management addresses objects by index (instance 1)")
				}
				ctx, cancel := remoteContext(cmd.Context(), a)
				defer cancel()
				r, err := dialRemote(ctx, a, args[0], tcp, nat, &sec, &rf)
				if err != nil {
					return err
				}
				defer r.close()
				idx, err := r.objectIndex(ctx, obj)
				if err != nil {
					return err
				}
				b, err := r.d.PropertyWrite(ctx, idx, uint8(pid), start, count, data)
				if err != nil {
					return err
				}
				return a.emit("property write", propertyResult{Server: args[0], Device: r.addr.String(), ObjectIndex: &idx, Secured: r.secured,
					ObjectType: obj, Instance: 1, PropertyID: uint8(pid), Start: start, Count: count, Data: strings.ToUpper(hex.EncodeToString(b)), Printable: printable(b)})
			}
			if rf.connected || rf.toolKey != "" {
				return errs.Invalid("--connected and --tool-key need --device")
			}
			c, err := dialDevMgmt(cmd, a, args[0], tcp, nat, &sec)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()
			if err := c.PropWrite(cmd.Context(), obj, instance, uint8(pid), start, count, data); err != nil {
				return err
			}
			return a.emit("property write", propertyResult{Server: args[0], ObjectType: obj, Instance: instance, PropertyID: uint8(pid), Start: start, Count: count, Data: strings.ToUpper(hex.EncodeToString(data))})
		},
	}
	write.Flags().Uint8Var(&instance, "instance", 1, "Object instance")
	write.Flags().Uint16Var(&start, "start", 1, "First element")
	write.Flags().Uint8Var(&count, "count", 1, "Number of elements (1..15)")
	write.Flags().BoolVar(&tcp, "tcp", false, "Connect over TCP")
	write.Flags().BoolVar(&nat, "nat", false, "Announce the route back endpoint (through NAT)")
	sec.add(write.Flags())
	rf.add(write.Flags())
	return write
}
