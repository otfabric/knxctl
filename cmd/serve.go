// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/device"
	"github.com/otfabric/go-knx/server"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

// serveConfig is the JSON configuration of knxctl serve (docs/serve.md).
type serveConfig struct {
	Name    string         `json:"name"`
	Address string         `json:"address"`
	Tunnels []string       `json:"tunnels"`
	Devices []deviceConfig `json:"devices"`
}

type deviceConfig struct {
	Name    string         `json:"name"`
	Address string         `json:"address"`
	Objects []objectConfig `json:"objects"`
}

type objectConfig struct {
	Name   string   `json:"name"`
	DPT    string   `json:"dpt"`
	Flags  string   `json:"flags"`
	Groups []string `json:"groups"`
	Value  string   `json:"value,omitempty"`
}

// parseObject parses --object group[+group...]:dpt[:flags][=value].
func parseObject(s string) (objectConfig, error) {
	spec, value, _ := strings.Cut(s, "=")
	parts := strings.Split(spec, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return objectConfig{}, errs.Invalid("--object %q: want group[+group]:dpt[:flags][=value]", s)
	}
	o := objectConfig{Name: parts[0], DPT: parts[1], Flags: "CRWU", Groups: strings.Split(parts[0], "+"), Value: value}
	if len(parts) == 3 {
		o.Flags = parts[2]
	}
	return o, nil
}

func buildDevice(dc deviceConfig) (*device.Device, error) {
	ia, err := knx.ParseIndividualAddress(dc.Address)
	if err != nil {
		return nil, errs.Invalid("device %q: %v", dc.Name, err)
	}
	d := device.New(dc.Name, ia)
	for _, oc := range dc.Objects {
		o, err := device.NewGroupObject(oc.Name, oc.DPT, oc.Flags, oc.Groups...)
		if err != nil {
			return nil, errs.Invalid("object %q: %v", oc.Name, err)
		}
		if oc.Value != "" {
			v, err := o.Type.Parse(oc.Value)
			if err != nil {
				return nil, errs.Invalid("object %q: %v", oc.Name, err)
			}
			if err := o.Set(v); err != nil {
				return nil, errs.Invalid("object %q: %v", oc.Name, err)
			}
		}
		d.Add(o)
	}
	return d, nil
}

func newServeCmd(a *app) *cobra.Command {
	var listenAddr, name, addr, config, devAddr string
	var tunnels, objects []string
	var routing, noDiscovery bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run a simulated KNXnet/IP server with virtual group objects",
		Long: `Run a KNXnet/IP server (discovery, description, tunnelling, optionally
routing) with a virtual KNX network behind it: telegrams of one tunnelling
client reach the other clients and the virtual group objects, which store
written values and answer reads according to their flags. Every telegram is
printed. This is a simulator for development and tests; it does not emulate a
physical KNX line or device management.

Group objects come from --object group[+group]:dpt[:flags][=value] on a
device at --device-address, or from a JSON file (--config, docs/serve.md).`,
		Example: `  knxctl serve --object 1/2/3:1.001:CRWU=off --object 1/2/4:9.001:CRT=21.5
  knxctl serve --config site.json --routing --interface eth0
  knxctl serve --listen 127.0.0.1:3671 --no-discovery --tunnels 1.1.10,1.1.11`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := serveConfig{Name: name, Address: addr, Tunnels: tunnels}
			if config != "" {
				b, err := os.ReadFile(config)
				if err != nil {
					return errs.New(errs.KindInvalidInput, errs.CodeInvalidInput, "--config: %v", err)
				}
				if err := json.Unmarshal(b, &cfg); err != nil {
					return errs.Invalid("--config %s: %v", config, err)
				}
				if cmd.Flags().Changed("name") {
					cfg.Name = name
				}
			}
			if len(objects) > 0 {
				dc := deviceConfig{Name: "virtual", Address: devAddr}
				for _, s := range objects {
					o, err := parseObject(s)
					if err != nil {
						return err
					}
					dc.Objects = append(dc.Objects, o)
				}
				cfg.Devices = append(cfg.Devices, dc)
			}
			opts := []server.Option{server.WithListenAddr(listenAddr), server.WithLogger(a.logger), server.WithRouting(routing), server.WithDiscovery(!noDiscovery)}
			if cfg.Name != "" {
				opts = append(opts, server.WithName(cfg.Name))
			}
			if a.iface != "" {
				opts = append(opts, server.WithInterface(a.iface))
			}
			if cfg.Address != "" {
				ia, err := knx.ParseIndividualAddress(cfg.Address)
				if err != nil {
					return errs.Invalid("--address: %v", err)
				}
				opts = append(opts, server.WithIndividualAddress(ia))
			}
			if len(cfg.Tunnels) > 0 {
				var ias []knx.IndividualAddress
				for _, t := range cfg.Tunnels {
					ia, err := knx.ParseIndividualAddress(strings.TrimSpace(t))
					if err != nil {
						return errs.Invalid("--tunnels: %v", err)
					}
					ias = append(ias, ia)
				}
				opts = append(opts, server.WithTunnelAddresses(ias...))
			}
			s, err := server.New(opts...)
			if err != nil {
				return err
			}
			objs := map[knx.GroupAddress]*device.GroupObject{}
			for _, dc := range cfg.Devices {
				d, err := buildDevice(dc)
				if err != nil {
					return err
				}
				for _, o := range d.Objects() {
					for _, g := range o.Addresses {
						objs[g] = o
					}
				}
				s.AddDevice(d)
			}
			st := a.stream()
			s.Subscribe(func(t server.Telegram) {
				g, ok := t.Frame.Group()
				if !ok {
					return
				}
				svc, v, err := apdu.ParseGroupValue(t.Frame.TPDU)
				if err != nil {
					return
				}
				r := telegramRecord{Time: nowUTC(), Source: t.Frame.Source.String(), Group: g.String(), Service: serviceName(svc)}
				if svc != apdu.GroupValueReadService {
					r.Data, r.Small = fmt.Sprintf("%X", v.Data), v.Small
					if o := objs[g]; o != nil {
						r.DPT = o.Type.ID.String()
						if val, err := o.Type.Decode(v.Data); err == nil {
							r.Value, r.Text = jsonValue(val), o.Type.Format(val)
						}
					}
				}
				_ = st.Record("telegram", r.line(), r)
			})
			if err := s.Start(cmd.Context()); err != nil {
				return err
			}
			a.warn("serving on %s (%d devices; Ctrl-C to stop)", s.Addr(), len(cfg.Devices))
			<-cmd.Context().Done()
			return s.Close()
		},
	}
	f := cmd.Flags()
	f.StringVar(&listenAddr, "listen", ":3671", "UDP address to listen on")
	f.StringVar(&name, "name", "knxctl serve", "Friendly name")
	f.StringVar(&addr, "address", "", "Individual address of the server (default 15.15.0)")
	f.StringSliceVar(&tunnels, "tunnels", nil, "Tunnelling addresses, one per connection (default 15.15.250)")
	f.StringArrayVar(&objects, "object", nil, "Group object group[+group]:dpt[:flags][=value] (repeatable; flags default CRWU)")
	f.StringVar(&devAddr, "device-address", "1.1.1", "Individual address of the --object device")
	f.StringVar(&config, "config", "", "JSON configuration file (docs/serve.md)")
	f.BoolVar(&routing, "routing", false, "Route telegrams over IP multicast too")
	f.BoolVar(&noDiscovery, "no-discovery", false, "Do not join the multicast group for search requests")
	return cmd
}
