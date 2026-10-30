// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/datasecure"
	"github.com/otfabric/go-knx/device"
	"github.com/otfabric/go-knx/keyring"
	"github.com/otfabric/go-knx/secure"
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

// defaultFlags are the flags of an object that gives none.
const defaultFlags = "CRWU"

// parseObject parses --object group[+group...]:dpt[:flags][=value].
func parseObject(s string) (objectConfig, error) {
	spec, value, _ := strings.Cut(s, "=")
	parts := strings.Split(spec, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return objectConfig{}, errs.Invalid("--object %q: want group[+group]:dpt[:flags][=value]", s)
	}
	o := objectConfig{Name: parts[0], DPT: parts[1], Flags: defaultFlags, Groups: strings.Split(parts[0], "+"), Value: value}
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
		if oc.Flags == "" {
			oc.Flags = defaultFlags // as for --object; no flags would leave the object unlinked
		}
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
	var tunnels, objects, secureUsers []string
	var routing, noDiscovery, noTCP, concurrentBusmon bool
	var sec secureFlags
	var keyringHost string
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
			opts := []server.Option{server.WithListenAddr(listenAddr), server.WithLogger(a.logger), server.WithRouting(routing), server.WithDiscovery(!noDiscovery),
				server.WithTCP(!noTCP), server.WithConcurrentBusmonitor(concurrentBusmon)}
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
			secOpt, keys, err := serveSecure(&sec, secureUsers, keyringHost, &cfg)
			if err != nil {
				return err
			}
			if secOpt != nil {
				opts = append(opts, secOpt)
			}
			if routing {
				key, latency, ok, err := sec.backbone()
				if err != nil {
					return err
				}
				if ok {
					opts = append(opts, server.WithSecureRouting(key, latency))
				}
			} else if sec.backboneKey != "" {
				return errs.Invalid("--backbone-key needs --routing")
			}
			if len(cfg.Tunnels) > 0 && !cmd.Flags().Changed("tunnels") && config == "" {
				opts = append(opts, server.WithTunnelAddresses(parseAddrs(cfg.Tunnels)...))
			}
			if cfg.Address != "" && !cmd.Flags().Changed("address") && config == "" {
				opts = append(opts, server.WithIndividualAddress(knx.MustParseIndividualAddress(cfg.Address)))
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
				frame, secured := t.Frame, datasecure.IsSecure(t.Frame)
				if secured {
					if k, ok := keys[g]; ok {
						if plain, _, _, err := datasecure.Open(datasecure.Key(k), frame); err == nil {
							frame = plain
						}
					}
				}
				svc, v, err := apdu.ParseGroupValue(frame.TPDU)
				if err != nil {
					if secured {
						r := telegramRecord{Time: nowUTC(), Source: t.Frame.Source.String(), Group: g.String(), Service: "secure", Secure: true}
						_ = st.Record("telegram", r.line(), r)
					}
					return
				}
				r := telegramRecord{Time: nowUTC(), Source: t.Frame.Source.String(), Group: g.String(), Service: serviceName(svc), Secure: secured}
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
	f.BoolVar(&noTCP, "no-tcp", false, "Do not accept KNXnet/IP over TCP")
	f.BoolVar(&concurrentBusmon, "concurrent-busmonitor", false, "Let busmonitor connections coexist with tunnels (simulators)")
	f.StringArrayVar(&secureUsers, "secure-user", nil, "KNX IP Secure tunnelling user id:password[:tunnel address] (repeatable; makes the server secure)")
	f.StringVar(&sec.devicePassword, "device-password", envOr("KNXCTL_DEVICE_PASSWORD", ""), "Device authentication password of the secure server (env KNXCTL_DEVICE_PASSWORD)")
	f.StringVar(&sec.keyringPath, "keyring", envOr("KNXCTL_KEYRING", ""), "Serve the secure tunnelling users of a keyring host (env KNXCTL_KEYRING)")
	f.StringVar(&sec.keyringPassword, "keyring-password", envOr("KNXCTL_KEYRING_PASSWORD", ""), "Password of the keyring (env KNXCTL_KEYRING_PASSWORD)")
	f.StringVar(&sec.backboneKey, "backbone-key", envOr("KNXCTL_BACKBONE_KEY", ""), "KNX IP Secure routing backbone key for --routing, 32 hex digits (default: the keyring's) (env KNXCTL_BACKBONE_KEY)")
	f.StringVar(&keyringHost, "keyring-host", "", "Keyring device whose tunnelling interfaces to serve (default: the host of the first secure interface)")
	return cmd
}

func parseAddrs(ss []string) []knx.IndividualAddress {
	var out []knx.IndividualAddress
	for _, s := range ss {
		out = append(out, knx.MustParseIndividualAddress(s))
	}
	return out
}

// serveSecure builds the secure option of serve from --secure-user or a
// keyring host, and returns the keyring's group keys (to print Data
// Secure telegrams). A keyring host also gives the server address and
// tunnels when the configuration has none.
func serveSecure(sec *secureFlags, users []string, host string, cfg *serveConfig) (server.Option, map[knx.GroupAddress]keyring.Key, error) {
	kr, err := sec.keyring()
	if err != nil {
		return nil, nil, err
	}
	var keys map[knx.GroupAddress]keyring.Key
	if kr != nil {
		keys = kr.GroupKeys
	}
	su := map[uint8]server.SecureUser{}
	devPassword := sec.devicePassword
	if kr != nil && len(users) == 0 {
		var hostIA knx.IndividualAddress
		if host != "" {
			if hostIA, err = knx.ParseIndividualAddress(host); err != nil {
				return nil, nil, errs.Invalid("--keyring-host: %v", err)
			}
		} else {
			for _, itf := range kr.Interfaces {
				if itf.Type == keyring.Tunneling && itf.UserID != 0 && itf.HasHost {
					hostIA = itf.Host
					break
				}
			}
		}
		tunnels := kr.Tunnels(hostIA)
		for _, itf := range tunnels {
			if itf.UserID == 0 {
				continue
			}
			k, err := secure.UserPasswordKey(itf.Password)
			if err != nil {
				return nil, nil, errs.Invalid("keyring user %d: %v", itf.UserID, err)
			}
			su[itf.UserID] = server.SecureUser{Key: k, Address: itf.IndividualAddress}
			if devPassword == "" {
				devPassword = itf.Authentication
			}
		}
		if d := kr.Device(hostIA); d != nil && d.ManagementPassword != "" && len(su) > 0 {
			k, err := secure.UserPasswordKey(d.ManagementPassword)
			if err != nil {
				return nil, nil, errs.Invalid("keyring device %s: %v", hostIA, err)
			}
			su[1] = server.SecureUser{Key: k} // the management user
			if d.Authentication != "" {
				devPassword = d.Authentication
			}
		}
		if len(su) == 0 {
			return nil, keys, nil // a keyring for Data Secure only
		}
		if len(cfg.Tunnels) == 0 {
			for _, itf := range tunnels {
				cfg.Tunnels = append(cfg.Tunnels, itf.IndividualAddress.String())
			}
		}
		if cfg.Address == "" {
			cfg.Address = hostIA.String()
		}
	}
	for _, u := range users {
		parts := strings.SplitN(u, ":", 3)
		if len(parts) < 2 {
			return nil, nil, errs.Invalid("--secure-user %q: want id:password[:tunnel address]", redact(u))
		}
		id, err := strconv.Atoi(parts[0])
		if err != nil || id < 1 || id > 127 {
			return nil, nil, errs.Invalid("--secure-user: user id %q (1..127)", parts[0])
		}
		k, err := secure.UserPasswordKey(parts[1])
		if err != nil {
			return nil, nil, errs.Invalid("--secure-user %d: %v", id, err)
		}
		user := server.SecureUser{Key: k}
		if len(parts) == 3 {
			if user.Address, err = knx.ParseIndividualAddress(parts[2]); err != nil {
				return nil, nil, errs.Invalid("--secure-user %d: %v", id, err)
			}
		}
		su[uint8(id)] = user
	}
	if len(su) == 0 {
		return nil, keys, nil
	}
	if devPassword == "" {
		return nil, nil, errs.Invalid("a secure server needs --device-password (or a keyring)")
	}
	dev, err := secure.DeviceAuthenticationCode(devPassword)
	if err != nil {
		return nil, nil, errs.Invalid("--device-password: %v", err)
	}
	return server.WithSecure(dev, su), keys, nil
}

// redact hides the password of an id:password[:address] argument.
func redact(s string) string {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) >= 2 {
		parts[1] = "***"
	}
	return strings.Join(parts, ":")
}
