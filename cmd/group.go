// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/dpt"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

func newGroupCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Group communication: read, write, respond, answer, listen",
	}
	cmd.AddCommand(newGroupWriteCmd(a, false), newGroupWriteCmd(a, true), newGroupReadCmd(a), newGroupListenCmd(a), newGroupAnswerCmd(a))
	return cmd
}

type writeResult struct {
	Group   string `json:"group"`
	Service string `json:"service"`
	Data    string `json:"data"`
	Small   bool   `json:"small"`
	DPT     string `json:"dpt,omitempty"`
	Via     string `json:"via"`
}

func (w writeResult) Text() string {
	s := fmt.Sprintf("%s %s %s", w.Service, w.Group, w.Data)
	if w.DPT != "" {
		s += " (DPT " + w.DPT + ")"
	}
	return s + " sent via " + w.Via
}

func via(l *linkFlags) string {
	if l.routing {
		return "routing " + l.group
	}
	return "tunnel " + l.gateway
}

func newGroupWriteCmd(a *app, respond bool) *cobra.Command {
	var lf linkFlags
	var dptID, raw string
	var small bool
	use, short, svc := "write group value", "Send A_GroupValue_Write (transmits a telegram)", "write"
	example := `  knxctl group write 1/2/3 on --gateway 192.168.1.10
  knxctl group write 1/2/4 21.5 --dpt 9.001 --gateway 192.168.1.10
  knxctl group write 1/2/5 --raw 0C33 --routing --source 1.1.250`
	if respond {
		use, short, svc = "respond group value", "Send A_GroupValue_Response (transmits a telegram)", "response"
		example = "  knxctl group respond 1/2/4 21.5 --dpt 9.001 --gateway 192.168.1.10"
	}
	cmd := &cobra.Command{
		Use:     strings.Replace(use, "group", "<group>", 1),
		Short:   short,
		Long:    short + ".\n\nThe value is parsed with --dpt (see knxctl dpt list); without it a boolean\n(on/off, true/false, 1/0) is sent as DPT 1. --raw sends octets as given.",
		Example: example,
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := parseGroup(args[0])
			if err != nil {
				return err
			}
			var ty *dpt.Type
			if dptID != "" {
				if ty, err = lookupDPT(dptID); err != nil {
					return err
				}
			}
			value := ""
			switch {
			case raw != "" && len(args) == 2:
				return errs.Invalid("give either a value or --raw")
			case raw == "" && len(args) != 2:
				return errs.New(errs.KindUsage, errs.CodeInvalidInput, "a value or --raw is required")
			case len(args) == 2:
				value = args[1]
			}
			gv, err := parseValue(ty, raw, small, value)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), a.timeout)
			defer cancel()
			c, err := lf.open(ctx, a)
			if err != nil {
				return err
			}
			defer c.Close()
			if respond {
				err = c.GroupRespond(ctx, g, gv)
			} else {
				err = c.GroupWrite(ctx, g, gv)
			}
			if err != nil {
				return err
			}
			r := writeResult{Group: g.String(), Service: svc, Data: fmt.Sprintf("%X", gv.Data), Small: gv.Small, Via: via(&lf)}
			if ty != nil {
				r.DPT = ty.ID.String()
			}
			return a.emit("group "+svc, r)
		},
	}
	lf.add(cmd)
	cmd.Flags().StringVarP(&dptID, "dpt", "t", "", "Datapoint type of the value (9.001)")
	cmd.Flags().StringVar(&raw, "raw", "", "Send these octets (hexadecimal) instead of a value")
	cmd.Flags().BoolVar(&small, "small", false, "With --raw: send one octet 00..3F in the APCI (6-bit value)")
	return cmd
}

type readResult struct {
	Group     string           `json:"group"`
	Responses []telegramRecord `json:"responses"`
}

func (r readResult) Text() string {
	if len(r.Responses) == 0 {
		return "no response from " + r.Group
	}
	lines := make([]string, len(r.Responses))
	for i, t := range r.Responses {
		lines[i] = t.line()
	}
	return strings.Join(lines, "\n")
}

func newGroupReadCmd(a *app) *cobra.Command {
	var lf linkFlags
	var dptID string
	var all time.Duration
	cmd := &cobra.Command{
		Use:   "read <group>",
		Short: "Send A_GroupValue_Read and print the response (transmits a telegram)",
		Long: `Send A_GroupValue_Read and print the first A_GroupValue_Response within
--timeout. A response is not guaranteed: the read times out (exit 5) when no
device answers. Several devices may answer the same address; --all collects
every response during the given window.`,
		Example: `  knxctl group read 1/2/4 --dpt 9.001 --gateway 192.168.1.10
  knxctl group read 1/2/4 --all 2s --gateway 192.168.1.10 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := parseGroup(args[0])
			if err != nil {
				return err
			}
			var ty *dpt.Type
			if dptID != "" {
				if ty, err = lookupDPT(dptID); err != nil {
					return err
				}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), a.timeout+all)
			defer cancel()
			c, err := lf.open(ctx, a)
			if err != nil {
				return err
			}
			defer c.Close()
			res := readResult{Group: g.String(), Responses: []telegramRecord{}}
			if all > 0 {
				rs, err := c.GroupReadAll(ctx, g, all)
				if err != nil {
					return err
				}
				for _, r := range rs {
					res.Responses = append(res.Responses, newTelegramRecord(r.Telegram, ty))
				}
				if len(rs) == 0 {
					_ = a.emit("group read", res)
					return errs.New(errs.KindTimeout, errs.CodeNoResponse, "no response to the read of %s within %v", g, all)
				}
				return a.emit("group read", res)
			}
			rctx, rcancel := context.WithTimeout(ctx, a.timeout)
			defer rcancel()
			r, err := c.GroupRead(rctx, g)
			if err != nil {
				return err
			}
			res.Responses = append(res.Responses, newTelegramRecord(r.Telegram, ty))
			return a.emit("group read", res)
		},
	}
	lf.add(cmd)
	cmd.Flags().StringVarP(&dptID, "dpt", "t", "", "Datapoint type to decode the value with (9.001)")
	cmd.Flags().DurationVar(&all, "all", 0, "Collect every response during this window")
	return cmd
}

// listen streams group telegrams until count, duration or interruption.
func listen(ctx context.Context, a *app, lf *linkFlags, groups []knx.GroupAddress, m dptMap, def *dpt.Type, count int, duration time.Duration) error {
	if duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}
	octx, ocancel := context.WithTimeout(ctx, a.timeout)
	c, err := lf.open(octx, a)
	ocancel()
	if err != nil {
		return err
	}
	defer c.Close()
	sub := c.Subscribe(1024, groups...)
	a.warn("listening via %s (Ctrl-C to stop)", via(lf))
	st := a.stream()
	n := 0
	for {
		select {
		case <-ctx.Done():
			if d := sub.Dropped(); d > 0 {
				a.warn("%d telegrams dropped (output too slow)", d)
			}
			return nil
		case <-c.Done():
			if err := c.Err(); err != nil {
				return err
			}
			return nil
		case t, ok := <-sub.C():
			if !ok {
				return c.Err()
			}
			ty := m[t.Group]
			if ty == nil {
				ty = def
			}
			r := newTelegramRecord(t, ty)
			if err := st.Record("telegram", r.line(), r); err != nil {
				return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
			}
			n++
			if count > 0 && n >= count {
				return nil
			}
		}
	}
}

type listenFlags struct {
	maps     []string
	dptID    string
	count    int
	duration time.Duration
}

func (l *listenFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&l.maps, "map", nil, "Decode a group with a datapoint type: group=dpt (repeatable)")
	cmd.Flags().StringVarP(&l.dptID, "dpt", "t", "", "Decode every group with this datapoint type")
	cmd.Flags().IntVarP(&l.count, "count", "n", 0, "Stop after this many telegrams")
	cmd.Flags().DurationVar(&l.duration, "duration", 0, "Stop after this time")
}

func (l *listenFlags) resolve() (dptMap, *dpt.Type, error) {
	m, err := parseDPTMap(l.maps)
	if err != nil {
		return nil, nil, err
	}
	var def *dpt.Type
	if l.dptID != "" {
		if def, err = lookupDPT(l.dptID); err != nil {
			return nil, nil, err
		}
	}
	return m, def, nil
}

func newGroupListenCmd(a *app) *cobra.Command {
	var lf linkFlags
	var ls listenFlags
	cmd := &cobra.Command{
		Use:   "listen [group...]",
		Short: "Print the group telegrams to some or all groups (passive)",
		Example: `  knxctl group listen 1/2/3 1/2/4 --map 1/2/4=9.001 --gateway 192.168.1.10
  knxctl group listen --routing --count 10 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var groups []knx.GroupAddress
			for _, s := range args {
				g, err := parseGroup(s)
				if err != nil {
					return err
				}
				groups = append(groups, g)
			}
			m, def, err := ls.resolve()
			if err != nil {
				return err
			}
			return listen(cmd.Context(), a, &lf, groups, m, def, ls.count, ls.duration)
		},
	}
	lf.add(cmd)
	ls.add(cmd)
	return cmd
}

func newMonitorCmd(a *app) *cobra.Command {
	var lf linkFlags
	var ls listenFlags
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Print every group telegram (passive)",
		Long: `Print every group telegram seen through a tunnel (--gateway) or on the routing
multicast group (--routing). Nothing is sent onto the KNX network.`,
		Example: `  knxctl monitor --gateway 192.168.1.10 --map 1/2/4=9.001
  knxctl monitor --routing --interface eth0`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, def, err := ls.resolve()
			if err != nil {
				return err
			}
			return listen(cmd.Context(), a, &lf, nil, m, def, ls.count, ls.duration)
		},
	}
	lf.add(cmd)
	ls.add(cmd)
	return cmd
}

func newRoutingCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "routing", Short: "KNXnet/IP routing (IP multicast)"}
	var lf linkFlags
	var ls listenFlags
	mon := &cobra.Command{
		Use:     "monitor",
		Short:   "Print the routing indications on the multicast group (passive)",
		Example: "  knxctl routing monitor --interface eth0 --map 1/2/4=9.001",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			lf.routing = true
			m, def, err := ls.resolve()
			if err != nil {
				return err
			}
			return listen(cmd.Context(), a, &lf, nil, m, def, ls.count, ls.duration)
		},
	}
	mon.Flags().StringVar(&lf.group, "multicast", "224.0.23.12:3671", "Routing multicast group")
	ls.add(mon)
	cmd.AddCommand(mon)
	return cmd
}
