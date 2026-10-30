// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/otfabric/go-knx/discovery"
	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

// deviceRecord is a KNXnet/IP server found or described.
type deviceRecord struct {
	Endpoint        string            `json:"endpoint"`
	Name            string            `json:"name"`
	Address         string            `json:"individualAddress"`
	Medium          string            `json:"medium"`
	Serial          string            `json:"serial"`
	MAC             string            `json:"mac"`
	Multicast       string            `json:"multicast,omitempty"`
	ProgMode        bool              `json:"programmingMode"`
	Project         uint16            `json:"projectInstallation"`
	Services        map[string]uint8  `json:"services"`
	SecuredServices map[string]uint8  `json:"securedServices,omitempty"`
	Extended        bool              `json:"extended,omitempty"`
	Tunnels         []tunnelSlot      `json:"tunnels,omitempty"`
	Addresses       []string          `json:"knxAddresses,omitempty"`
	IP              map[string]string `json:"ip,omitempty"`
	Other           []string          `json:"otherBlocks,omitempty"`
}

type tunnelSlot struct {
	Address string `json:"address"`
	Free    bool   `json:"free"`
	Status  uint16 `json:"status"`
}

func newDeviceRecord(endpoint string, dibs knxnetip.DIBs) deviceRecord {
	r := deviceRecord{Endpoint: endpoint, Services: map[string]uint8{}}
	for _, d := range dibs {
		switch d := d.(type) {
		case knxnetip.DeviceInfo:
			r.Name, r.Address, r.Medium, r.Serial = d.Name, d.Address.String(), d.Medium.String(), d.SerialString()
			r.MAC = fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", d.MAC[0], d.MAC[1], d.MAC[2], d.MAC[3], d.MAC[4], d.MAC[5])
			r.ProgMode, r.Project = d.ProgrammingMode(), d.ProjectInstallation
			if d.Multicast.IsValid() && !d.Multicast.IsUnspecified() {
				r.Multicast = d.Multicast.String()
			}
		case knxnetip.ServiceFamilies:
			m := r.Services
			if d.Secured {
				r.SecuredServices = map[string]uint8{}
				m = r.SecuredServices
			}
			for _, f := range d.Families {
				m[f.ID.String()] = f.Version
			}
		case knxnetip.TunnellingInfo:
			for _, s := range d.Slots {
				r.Tunnels = append(r.Tunnels, tunnelSlot{Address: s.Address.String(), Free: s.Free(), Status: s.Status})
			}
		case knxnetip.KNXAddresses:
			for _, a := range d.Addresses {
				r.Addresses = append(r.Addresses, a.String())
			}
		case knxnetip.IPConfig:
			r.IP = map[string]string{"configuredIP": d.IP.String(), "configuredMask": d.Mask.String(), "configuredGateway": d.Gateway.String()}
		case knxnetip.IPCurrentConfig:
			if r.IP == nil {
				r.IP = map[string]string{}
			}
			r.IP["currentIP"], r.IP["currentMask"], r.IP["currentGateway"] = d.IP.String(), d.Mask.String(), d.Gateway.String()
		default:
			r.Other = append(r.Other, d.Type().String())
		}
	}
	return r
}

func servicesString(m map[string]uint8) string {
	order := []string{"Core", "DeviceManagement", "Tunnelling", "Routing", "RemoteLogging", "RemoteConfigurationAndDiagnosis", "ObjectServer", "Security"}
	var parts []string
	for _, k := range order {
		if v, ok := m[k]; ok {
			parts = append(parts, fmt.Sprintf("%s v%d", k, v))
		}
	}
	return strings.Join(parts, ", ")
}

func (r deviceRecord) text() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s  %q\n", r.Endpoint, r.Name)
	fmt.Fprintf(&sb, "  individual address %s, medium %s, serial %s, MAC %s\n", r.Address, r.Medium, r.Serial, r.MAC)
	if r.Multicast != "" {
		fmt.Fprintf(&sb, "  routing multicast %s\n", r.Multicast)
	}
	if r.ProgMode {
		sb.WriteString("  programming mode on\n")
	}
	fmt.Fprintf(&sb, "  services: %s\n", servicesString(r.Services))
	if len(r.SecuredServices) > 0 {
		fmt.Fprintf(&sb, "  secured services: %s\n", servicesString(r.SecuredServices))
	}
	for _, t := range r.Tunnels {
		free := "in use"
		if t.Free {
			free = "free"
		}
		fmt.Fprintf(&sb, "  tunnel %s %s\n", t.Address, free)
	}
	if len(r.Addresses) > 0 {
		fmt.Fprintf(&sb, "  KNX addresses: %s\n", strings.Join(r.Addresses, ", "))
	}
	for k, v := range r.IP {
		fmt.Fprintf(&sb, "  %s %s\n", k, v)
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

type discoverResult struct {
	Devices []deviceRecord `json:"devices"`
}

func (d discoverResult) Text() string {
	if len(d.Devices) == 0 {
		return "no KNXnet/IP server answered"
	}
	parts := make([]string, len(d.Devices))
	for i, r := range d.Devices {
		parts[i] = r.text()
	}
	return strings.Join(parts, "\n\n")
}

func (d discoverResult) Headers() []string {
	return []string{"ENDPOINT", "NAME", "ADDRESS", "MEDIUM", "SERIAL", "SERVICES"}
}

func (d discoverResult) Rows() [][]string {
	rows := make([][]string, len(d.Devices))
	for i, r := range d.Devices {
		rows[i] = []string{r.Endpoint, r.Name, r.Address, r.Medium, r.Serial, servicesString(r.Services)}
	}
	return rows
}

func newDiscoverCmd(a *app) *cobra.Command {
	var extended, nat, progMode bool
	var unicast, mac, service string
	var repeat int
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find KNXnet/IP servers with a search request",
		Long: `Send one search request to the KNXnet/IP multicast group 224.0.23.12:3671 (or,
with --unicast, to one server) and list the servers that answer within
--timeout. It is not a scan: one request (--repeat up to 3), no probing.

Multicast must reach the servers: on Docker, VPN or cloud networks it often
does not; use --unicast or describe instead.`,
		Example: `  knxctl discover
  knxctl discover --interface eth0 --timeout 2s
  knxctl discover --extended --format json
  knxctl discover --programming-mode
  knxctl discover --service tunnelling:2
  knxctl discover --unicast 192.168.1.10`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := []discovery.Option{discovery.WithTimeout(a.timeout), discovery.WithLogger(a.logger), discovery.WithRepeat(repeat), discovery.WithNAT(nat)}
			if a.iface != "" {
				opts = append(opts, discovery.WithInterface(a.iface))
			}
			if a.local != "" {
				opts = append(opts, discovery.WithLocalAddr(a.local))
			}
			if unicast != "" {
				opts = append(opts, discovery.WithUnicast(unicast))
			}
			params, err := searchParams(progMode, mac, service)
			if err != nil {
				return err
			}
			if extended || len(params) > 0 {
				opts = append(opts, discovery.WithExtended(params...))
			}
			rs, err := discovery.Search(cmd.Context(), opts...)
			if err != nil {
				return err
			}
			res := discoverResult{Devices: []deviceRecord{}}
			for _, r := range rs {
				d := newDeviceRecord(r.Endpoint().String(), r.DIBs)
				d.Extended = r.Extended
				res.Devices = append(res.Devices, d)
			}
			return a.emit("discover", res)
		},
	}
	cmd.Flags().BoolVar(&extended, "extended", false, "Send SEARCH_REQUEST_EXTENDED (KNXnet/IP core v2)")
	cmd.Flags().StringVar(&unicast, "unicast", "", "Directed search of one server, host[:port]")
	cmd.Flags().BoolVar(&nat, "nat", false, "Announce the route back endpoint (directed search through NAT)")
	cmd.Flags().IntVar(&repeat, "repeat", 1, "Send the search 1..3 times, one second apart")
	cmd.Flags().BoolVar(&progMode, "programming-mode", false, "Only servers in programming mode (extended search)")
	cmd.Flags().StringVar(&mac, "mac", "", "Only the server with this MAC address, aa:bb:cc:dd:ee:ff (extended search)")
	cmd.Flags().StringVar(&service, "service", "", "Only servers with this service family and at least this version, family[:version] (extended search; core, devmgmt, tunnelling, routing, security...)")
	return cmd
}

// familyNames are the service family names of --service.
var familyNames = map[string]knxnetip.ServiceFamilyID{
	"core": knxnetip.FamilyCore, "devmgmt": knxnetip.FamilyDeviceManagement, "device-management": knxnetip.FamilyDeviceManagement,
	"tunnelling": knxnetip.FamilyTunnelling, "tunneling": knxnetip.FamilyTunnelling, "routing": knxnetip.FamilyRouting,
	"remote-logging": knxnetip.FamilyRemoteLogging, "remote-config": knxnetip.FamilyRemoteConfig,
	"object-server": knxnetip.FamilyObjectServer, "security": knxnetip.FamilySecurity,
}

// searchParams builds the SEARCH_REQUEST_EXTENDED parameters of the
// filter flags.
func searchParams(progMode bool, mac, service string) ([]knxnetip.SearchParam, error) {
	var ps []knxnetip.SearchParam
	if progMode {
		ps = append(ps, knxnetip.SearchParam{Type: knxnetip.SRPProgrammingMode, Mandatory: true})
	}
	if mac != "" {
		hw, err := net.ParseMAC(mac)
		if err != nil || len(hw) != 6 {
			return nil, errs.Invalid("--mac %q: want aa:bb:cc:dd:ee:ff", mac)
		}
		ps = append(ps, knxnetip.SearchParam{Type: knxnetip.SRPMACAddress, Mandatory: true, Data: hw})
	}
	if service != "" {
		name, ver, hasVer := strings.Cut(strings.ToLower(service), ":")
		id, ok := familyNames[name]
		if !ok {
			return nil, errs.Invalid("--service %q: unknown service family", service)
		}
		v := 1
		if hasVer {
			n, err := strconv.Atoi(ver)
			if err != nil || n < 1 || n > 255 {
				return nil, errs.Invalid("--service %q: version 1..255", service)
			}
			v = n
		}
		ps = append(ps, knxnetip.SearchParam{Type: knxnetip.SRPService, Mandatory: true, Data: []byte{byte(id), byte(v)}})
	}
	return ps, nil
}

type describeResult struct {
	Device deviceRecord `json:"device"`
}

func (d describeResult) Text() string { return d.Device.text() }

func newDescribeCmd(a *app) *cobra.Command {
	var nat bool
	cmd := &cobra.Command{
		Use:     "describe host[:port]",
		Short:   "Read the self description of a KNXnet/IP server",
		Example: "  knxctl describe 192.168.1.10\n  knxctl describe knx-gw.local --format json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := []discovery.Option{discovery.WithTimeout(a.timeout), discovery.WithLogger(a.logger), discovery.WithNAT(nat)}
			if a.local != "" {
				opts = append(opts, discovery.WithLocalAddr(a.local))
			}
			dibs, err := discovery.Describe(cmd.Context(), args[0], opts...)
			if err != nil {
				return err
			}
			if _, ok := dibs.DeviceInfo(); !ok {
				return errs.New(errs.KindProtocol, errs.CodeMalformed, "the description of %s has no DEVICE_INFO", args[0])
			}
			return a.emit("describe", describeResult{Device: newDeviceRecord(args[0], dibs)})
		},
	}
	cmd.Flags().BoolVar(&nat, "nat", false, "Announce the route back endpoint (through NAT)")
	return cmd
}
