// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/cemi"
	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

// busmonRecord is a frame received on a busmonitor connection.
type busmonRecord struct {
	Time        string `json:"time"`
	Raw         string `json:"raw"`
	Status      string `json:"status,omitempty"`
	Timestamp   *int   `json:"timestamp,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Priority    string `json:"priority,omitempty"`
	HopCount    *int   `json:"hopCount,omitempty"`
	Service     string `json:"service,omitempty"`
	Data        string `json:"data,omitempty"`
	Error       string `json:"decodeError,omitempty"`
}

func (r busmonRecord) line() string {
	if r.Error != "" {
		return fmt.Sprintf("%s %s (undecodable: %s)", r.Time, r.Raw, r.Error)
	}
	s := fmt.Sprintf("%s %-9s -> %-9s %s", r.Time, r.Source, r.Destination, r.Service)
	if r.Data != "" {
		s += " " + r.Data
	}
	return s + "  [" + r.Raw + "]"
}

func newBusmonitorRecord(f *cemi.Raw) busmonRecord {
	r := busmonRecord{Time: nowUTC(), Raw: strings.ToUpper(hex.EncodeToString(f.Data))}
	for _, ai := range f.AddInfo {
		switch {
		case ai.Type == cemi.AddInfoBusmonitorStatus && len(ai.Data) == 1:
			r.Status = fmt.Sprintf("0x%02X", ai.Data[0])
		case ai.Type == cemi.AddInfoTimestamp && len(ai.Data) == 2:
			ts := int(ai.Data[0])<<8 | int(ai.Data[1])
			r.Timestamp = &ts
		}
	}
	ld, err := cemi.ParseTP1(f.Data)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	hop := int(ld.HopCount())
	r.Source, r.Destination, r.Priority, r.HopCount = ld.Source.String(), ld.DestinationString(), ld.Priority().String(), &hop
	if p, err := apdu.Parse(ld.TPDU); err == nil {
		r.Service = p.TPCI.Kind.String()
		if !p.TPCI.IsControl() {
			r.Service = p.APCI.String()
			data := p.Data
			if p.APCI.Has6BitData() && len(data) == 0 {
				data = []byte{p.Short}
			}
			r.Data = strings.ToUpper(hex.EncodeToString(data))
		}
	}
	return r
}

func newBusmonitorCmd(a *app) *cobra.Command {
	var nat, tcp bool
	var count int
	var duration time.Duration
	cmd := &cobra.Command{
		Use:   "busmonitor host[:port]",
		Short: "Monitor every frame on the bus through a busmonitor connection (passive)",
		Long: `Open a busmonitor tunnelling connection and print every frame the KNXnet/IP
server reports (L_Busmon.ind): the raw TP1 frame, decoded. Nothing is sent.

On a KNX interface a busmonitor connection is exclusive: while it is open the
interface refuses other tunnelling connections, and it cannot be opened while
another is connected.`,
		Example: "  knxctl busmonitor 192.168.1.10\n  knxctl busmonitor 192.168.1.10 --count 20 --format json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if duration > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, duration)
				defer cancel()
			}
			opts := []tunnel.Option{tunnel.WithLogger(a.logger), tunnel.WithConnectTimeout(a.timeout), tunnel.WithLayer(knxnetip.TunnelBusmonitor), tunnel.WithNAT(nat)}
			if tcp {
				opts = append(opts, tunnel.WithTCP())
			}
			if a.local != "" {
				opts = append(opts, tunnel.WithLocalAddr(a.local))
			}
			c, err := tunnel.Dial(ctx, args[0], opts...)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()
			a.warn("busmonitor on %s (Ctrl-C to stop)", args[0])
			st := a.stream()
			n := 0
			for count == 0 || n < count {
				f, err := c.Receive(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return err
				}
				raw, ok := f.(*cemi.Raw)
				if !ok || raw.MsgCode != cemi.LBusmonInd {
					continue
				}
				r := newBusmonitorRecord(raw)
				if err := st.Record("busmonitor", r.line(), r); err != nil {
					return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
				}
				n++
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&nat, "nat", false, "Announce the route back endpoint (through NAT)")
	cmd.Flags().BoolVar(&tcp, "tcp", false, "Connect over TCP")
	cmd.Flags().IntVarP(&count, "count", "n", 0, "Stop after this many frames")
	cmd.Flags().DurationVar(&duration, "duration", 0, "Stop after this time")
	return cmd
}
