// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/client"
	"github.com/otfabric/go-knx/dpt"
	"github.com/otfabric/go-knx/routing"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

// linkFlags select how a command reaches the KNX network.
type linkFlags struct {
	gateway  string
	routing  bool
	group    string
	nat      bool
	source   string
	tunnelIA string
}

func (l *linkFlags) add(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&l.gateway, "gateway", "g", envOr("KNXCTL_GATEWAY", ""), "KNXnet/IP server to tunnel through, host[:port] (env KNXCTL_GATEWAY)")
	f.BoolVar(&l.routing, "routing", false, "Use KNXnet/IP routing (IP multicast) instead of a tunnel")
	f.StringVar(&l.group, "multicast", "224.0.23.12:3671", "Routing multicast group")
	f.BoolVar(&l.nat, "nat", false, "Tunnel through NAT: announce the route back endpoint 0.0.0.0:0")
	f.StringVar(&l.source, "source", "", "Source individual address of sent telegrams (routing; a tunnel uses the server's)")
	f.StringVar(&l.tunnelIA, "tunnel-address", "", "Request this tunnelling address (extended CRI, tunnelling v2 servers)")
}

func (l *linkFlags) open(ctx context.Context, a *app) (*client.Client, error) {
	var copts []client.Option
	copts = append(copts, client.WithLogger(a.logger))
	if l.source != "" {
		ia, err := knx.ParseIndividualAddress(l.source)
		if err != nil {
			return nil, errs.Invalid("--source: %v", err)
		}
		copts = append(copts, client.WithSource(ia))
	}
	switch {
	case l.routing && l.gateway != "":
		return nil, errs.Invalid("--gateway and --routing exclude each other")
	case l.routing:
		ropts := []routing.Option{routing.WithGroup(l.group), routing.WithLogger(a.logger)}
		if a.iface != "" {
			ropts = append(ropts, routing.WithInterface(a.iface))
		}
		return client.ListenRouting(ctx, ropts, copts...)
	case l.gateway != "":
		topts := []tunnel.Option{tunnel.WithLogger(a.logger), tunnel.WithNAT(l.nat), tunnel.WithConnectTimeout(a.timeout)}
		if a.local != "" {
			topts = append(topts, tunnel.WithLocalAddr(a.local))
		}
		if l.tunnelIA != "" {
			ia, err := knx.ParseIndividualAddress(l.tunnelIA)
			if err != nil {
				return nil, errs.Invalid("--tunnel-address: %v", err)
			}
			topts = append(topts, tunnel.WithIndividualAddress(ia))
		}
		return client.DialTunnel(ctx, l.gateway, topts, copts...)
	}
	return nil, errs.New(errs.KindUsage, errs.CodeInvalidInput, "a KNXnet/IP server (--gateway host) or --routing is required")
}

// dptMap maps group addresses to datapoint types for decoding.
type dptMap map[knx.GroupAddress]*dpt.Type

func parseDPTMap(entries []string) (dptMap, error) {
	m := dptMap{}
	for _, e := range entries {
		ga, id, ok := strings.Cut(e, "=")
		if !ok {
			return nil, errs.Invalid("--map %q: want group=dpt (1/2/3=9.001)", e)
		}
		g, err := knx.ParseGroupAddress(ga)
		if err != nil {
			return nil, errs.Invalid("--map %q: %v", e, err)
		}
		t, err := dpt.Lookup(id)
		if err != nil {
			return nil, errs.Invalid("--map %q: %v", e, err)
		}
		m[g] = t
	}
	return m, nil
}

func parseGroup(s string) (knx.GroupAddress, error) {
	g, err := knx.ParseGroupAddress(s)
	if err != nil {
		return 0, errs.New(errs.KindInvalidInput, errs.CodeInvalidAddress, "%v", err)
	}
	return g, nil
}

func lookupDPT(id string) (*dpt.Type, error) {
	t, err := dpt.Lookup(id)
	if err != nil {
		return nil, errs.New(errs.KindInvalidInput, errs.CodeInvalidInput, "datapoint type %q: %v (see knxctl dpt list)", id, err)
	}
	return t, nil
}

// telegramRecord is a group telegram as knxctl prints it.
type telegramRecord struct {
	Time    string `json:"time"`
	Source  string `json:"source"`
	Group   string `json:"group"`
	Service string `json:"service"`
	Data    string `json:"data,omitempty"`
	Small   bool   `json:"small,omitempty"`
	DPT     string `json:"dpt,omitempty"`
	Value   any    `json:"value,omitempty"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"decodeError,omitempty"`
}

func serviceName(s apdu.APCI) string {
	switch s {
	case apdu.GroupValueReadService:
		return "read"
	case apdu.GroupValueResponseService:
		return "response"
	case apdu.GroupValueWriteService:
		return "write"
	}
	return s.String()
}

func newTelegramRecord(t client.Telegram, ty *dpt.Type) telegramRecord {
	r := telegramRecord{Time: t.Time.UTC().Format(time.RFC3339Nano), Source: t.Source.String(), Group: t.Group.String(), Service: serviceName(t.Service)}
	if t.Service != apdu.GroupValueReadService {
		r.Data, r.Small = strings.ToUpper(hex.EncodeToString(t.Value.Data)), t.Value.Small
		if ty != nil {
			r.DPT = ty.ID.String()
			if v, err := ty.Decode(t.Value.Data); err == nil {
				r.Value, r.Text = jsonValue(v), ty.Format(v)
			} else {
				r.Error = err.Error()
			}
		}
	}
	return r
}

func (r telegramRecord) line() string {
	s := fmt.Sprintf("%s %-9s -> %-9s %-8s", r.Time, r.Source, r.Group, r.Service)
	if r.Data != "" {
		s += " " + r.Data
	}
	if r.Text != "" {
		s += " (" + r.Text + ")"
	}
	if r.Error != "" {
		s += " (undecodable: " + r.Error + ")"
	}
	return s
}

// jsonValue makes a decoded value JSON friendly: structured values keep
// their fields, the others are as decoded.
func jsonValue(v any) any {
	switch x := v.(type) {
	case dpt.Enum:
		return map[string]any{"value": x.Value, "name": x.Name}
	case dpt.TimeOfDay, dpt.Date, dpt.DateTime:
		return fmt.Sprint(x)
	case dpt.RGB, dpt.RGBW:
		return fmt.Sprint(x)
	}
	return v
}

// parseValue parses a value for a write: with a datapoint type its text
// form, without one a boolean for DPT 1.
func parseValue(ty *dpt.Type, raw string, small bool, value string) (apdu.GroupValue, error) {
	if raw != "" {
		b, err := hex.DecodeString(strings.ReplaceAll(strings.TrimPrefix(raw, "0x"), " ", ""))
		if err != nil || len(b) == 0 {
			return apdu.GroupValue{}, errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "--raw %q: want hexadecimal octets", raw)
		}
		if small && (len(b) != 1 || b[0] > 0x3F) {
			return apdu.GroupValue{}, errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "--small takes one octet 00..3F")
		}
		return apdu.GroupValue{Data: b, Small: small}, nil
	}
	if ty == nil {
		t, _ := dpt.Lookup("1.001")
		ty = t
	}
	v, err := ty.Parse(value)
	if err != nil {
		return apdu.GroupValue{}, errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "%v", err)
	}
	b, err := ty.Encode(v)
	if err != nil {
		return apdu.GroupValue{}, errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "%v", err)
	}
	return apdu.GroupValue{Data: b, Small: ty.Small()}, nil
}
