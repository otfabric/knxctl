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
	Server     string `json:"server"`
	ObjectType uint16 `json:"objectType"`
	Instance   uint8  `json:"instance"`
	PropertyID uint8  `json:"propertyId"`
	Start      uint16 `json:"start"`
	Count      uint8  `json:"count"`
	Data       string `json:"data"`
	Printable  string `json:"text,omitempty"`
}

func (r propertyResult) Text() string {
	s := fmt.Sprintf("object 0x%04X/%d property %d [%d..%d]: %s", r.ObjectType, r.Instance, r.PropertyID, r.Start, int(r.Start)+int(r.Count)-1, r.Data)
	if r.Printable != "" {
		s += fmt.Sprintf(" (%q)", r.Printable)
	}
	return s
}

func newPropertyCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "property", Short: "Interface object properties of a KNXnet/IP server (device management)"}
	var instance, count uint8
	var start uint16
	var tcp, nat bool
	read := &cobra.Command{
		Use:   "read host[:port] <object> <pid>",
		Short: "Read a property with local device management (M_PropRead)",
		Long: `Open a device management connection and read count elements of a property
from element start (start 0 reads the number of elements). The object is a type
number or device (0x0000), router (0x0006) or knxnetip (0x000B, the KNXnet/IP
parameter object). Nothing is sent onto the KNX network.`,
		Example: `  knxctl property read 192.168.1.10 device 11          # serial number
  knxctl property read 192.168.1.10 knxnetip 76 --count 30   # friendly name
  knxctl property read 192.168.1.10 knxnetip 53 --start 0     # number of tunnelling addresses`,
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
			opts := []tunnel.Option{tunnel.WithLogger(a.logger), tunnel.WithConnectTimeout(a.timeout), tunnel.WithNAT(nat)}
			if tcp {
				opts = append(opts, tunnel.WithTCP())
			}
			c, err := devmgmt.Dial(cmd.Context(), args[0], opts...)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()
			b, err := c.PropRead(cmd.Context(), obj, instance, uint8(pid), start, count)
			if err != nil {
				return err
			}
			r := propertyResult{Server: args[0], ObjectType: obj, Instance: instance, PropertyID: uint8(pid), Start: start, Count: count, Data: strings.ToUpper(hex.EncodeToString(b))}
			printable := len(b) > 0
			for _, c := range b {
				if c != 0 && (c < 0x20 || c > 0x7E) {
					printable = false
				}
			}
			if printable {
				r.Printable = strings.TrimRight(string(b), "\x00")
			}
			return a.emit("property read", r)
		},
	}
	read.Flags().Uint8Var(&instance, "instance", 1, "Object instance")
	read.Flags().Uint16Var(&start, "start", 1, "First element (0: the number of elements)")
	read.Flags().Uint8Var(&count, "count", 1, "Number of elements (1..15)")
	read.Flags().BoolVar(&tcp, "tcp", false, "Connect over TCP")
	read.Flags().BoolVar(&nat, "nat", false, "Announce the route back endpoint (through NAT)")
	cmd.AddCommand(read)
	return cmd
}
