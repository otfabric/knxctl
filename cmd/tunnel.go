// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/discovery"
	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

type tunnelResult struct {
	Server     string `json:"server"`
	Channel    uint8  `json:"channel"`
	Address    string `json:"individualAddress"`
	Local      string `json:"local"`
	ConnectMs  int64  `json:"connectMs"`
	Held       string `json:"held,omitempty"`
	Heartbeats bool   `json:"heartbeatsOk"`
}

func (t tunnelResult) Text() string {
	s := fmt.Sprintf("connected to %s: channel %d, address %s (local %s, %d ms)", t.Server, t.Channel, t.Address, t.Local, t.ConnectMs)
	if t.Held != "" {
		s += fmt.Sprintf("\nheld %s, connection alive: %t", t.Held, t.Heartbeats)
	}
	return s + "\ndisconnected"
}

func newTunnelCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "tunnel", Short: "KNXnet/IP tunnelling connections"}
	var hold time.Duration
	var nat, tcp bool
	var heartbeat time.Duration
	var ia string
	var sec secureFlags
	connect := &cobra.Command{
		Use:   "connect host[:port]",
		Short: "Open a tunnelling connection, report it and disconnect",
		Long: `Open a tunnelling connection, print the channel and tunnelling address, keep it
for --hold (sending heartbeats every --heartbeat) and disconnect. No telegram
is sent onto the KNX network.`,
		Example: "  knxctl tunnel connect 192.168.1.10\n  knxctl tunnel connect 192.168.1.10 --hold 2m --heartbeat 30s",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := []tunnel.Option{tunnel.WithLogger(a.logger), tunnel.WithNAT(nat), tunnel.WithConnectTimeout(a.timeout), tunnel.WithHeartbeat(heartbeat, tunnel.DefaultHeartbeatTimeout, tunnel.DefaultHeartbeatAttempts)}
			if a.local != "" {
				opts = append(opts, tunnel.WithLocalAddr(a.local))
			}
			if tcp {
				opts = append(opts, tunnel.WithTCP())
			}
			if ia != "" {
				addr, err := knx.ParseIndividualAddress(ia)
				if err != nil {
					return errs.Invalid("--tunnel-address: %v", err)
				}
				opts = append(opts, tunnel.WithIndividualAddress(addr))
			}
			opts, err := sec.tunnelOptions(opts, ia)
			if err != nil {
				return err
			}
			start := time.Now()
			c, err := tunnel.Dial(cmd.Context(), args[0], opts...)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()
			r := tunnelResult{Server: c.Server().String(), Channel: c.Channel(), Address: c.IndividualAddress().String(), Local: c.LocalAddr().String(), ConnectMs: time.Since(start).Milliseconds(), Heartbeats: true}
			if hold > 0 {
				select {
				case <-time.After(hold):
				case <-c.Done():
				case <-cmd.Context().Done():
				}
				r.Held, r.Heartbeats = hold.String(), c.State() == knx.StateConnected
			}
			return a.emit("tunnel connect", r)
		},
	}
	connect.Flags().DurationVar(&hold, "hold", 0, "Keep the connection this long before disconnecting")
	connect.Flags().DurationVar(&heartbeat, "heartbeat", tunnel.DefaultHeartbeatInterval, "Heartbeat interval while held")
	connect.Flags().BoolVar(&nat, "nat", false, "Announce the route back endpoint (through NAT)")
	connect.Flags().BoolVar(&tcp, "tcp", false, "Connect over TCP (KNXnet/IP core v2)")
	connect.Flags().StringVar(&ia, "tunnel-address", "", "Request this tunnelling address (tunnelling v2 servers)")
	sec.add(connect.Flags())
	cmd.AddCommand(connect)
	return cmd
}

type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Ms     int64  `json:"ms"`
}

type diagnoseResult struct {
	Server string  `json:"server"`
	Checks []check `json:"checks"`
	OK     bool    `json:"ok"`
}

func (d diagnoseResult) Text() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diagnose %s\n", d.Server)
	for _, c := range d.Checks {
		mark := "ok  "
		if !c.OK {
			mark = "FAIL"
		}
		fmt.Fprintf(&sb, "  [%s] %-22s %s (%d ms)\n", mark, c.Name, c.Detail, c.Ms)
	}
	if d.OK {
		sb.WriteString("all checks passed")
	} else {
		sb.WriteString("some checks failed")
	}
	return sb.String()
}

func (d diagnoseResult) Headers() []string { return []string{"CHECK", "RESULT", "MS", "DETAIL"} }

func (d diagnoseResult) Rows() [][]string {
	rows := make([][]string, len(d.Checks))
	for i, c := range d.Checks {
		res := "ok"
		if !c.OK {
			res = "FAIL"
		}
		rows[i] = []string{c.Name, res, fmt.Sprint(c.Ms), c.Detail}
	}
	return rows
}

func newDiagnoseCmd(a *app) *cobra.Command {
	var nat bool
	cmd := &cobra.Command{
		Use:   "diagnose host[:port]",
		Short: "Check a KNXnet/IP server: description, services, tunnelling, heartbeat",
		Long: `Run a sequence of checks against a KNXnet/IP server: directed search,
description request, supported service families, tunnelling connection,
connection state (heartbeat) and disconnection. No telegram is sent onto the KNX
network. Exits 1 when a check fails.`,
		Example: "  knxctl diagnose 192.168.1.10\n  knxctl diagnose 192.168.1.10 --format json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			res := diagnoseResult{Server: args[0], OK: true}
			run := func(name string, f func() (string, error)) bool {
				start := time.Now()
				detail, err := f()
				c := check{Name: name, OK: err == nil, Detail: detail, Ms: time.Since(start).Milliseconds()}
				if err != nil {
					c.Detail = err.Error()
					res.OK = false
				}
				res.Checks = append(res.Checks, c)
				return err == nil
			}
			dopts := []discovery.Option{discovery.WithTimeout(a.timeout), discovery.WithNAT(nat), discovery.WithLogger(a.logger)}
			run("directed search", func() (string, error) {
				rs, err := discovery.Search(ctx, append(dopts, discovery.WithUnicast(args[0]))...)
				if err != nil {
					return "", err
				}
				if len(rs) == 0 {
					return "", fmt.Errorf("no SEARCH_RESPONSE (servers without directed search do not answer)")
				}
				return fmt.Sprintf("%q at %s", rs[0].Device().Name, rs[0].Endpoint()), nil
			})
			var dibs knxnetip.DIBs
			if !run("description", func() (string, error) {
				var err error
				if dibs, err = discovery.Describe(ctx, args[0], dopts...); err != nil {
					return "", err
				}
				d, ok := dibs.DeviceInfo()
				if !ok {
					return "", fmt.Errorf("no DEVICE_INFO")
				}
				return fmt.Sprintf("%q, %s, medium %s", d.Name, d.Address, d.Medium), nil
			}) {
				_ = a.emit("diagnose", res)
				return errs.ErrFindings
			}
			run("tunnelling service", func() (string, error) {
				f, _ := dibs.SupportedServices()
				v, ok := f.Version(knxnetip.FamilyTunnelling)
				if !ok {
					return "", fmt.Errorf("tunnelling is not among the supported services")
				}
				return fmt.Sprintf("tunnelling v%d", v), nil
			})
			var conn *tunnel.Conn
			run("tunnel connect", func() (string, error) {
				var err error
				conn, err = tunnel.Dial(ctx, args[0], tunnel.WithNAT(nat), tunnel.WithConnectTimeout(a.timeout), tunnel.WithLogger(a.logger),
					tunnel.WithHeartbeat(500*time.Millisecond, a.timeout, 1))
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("channel %d, address %s", conn.Channel(), conn.IndividualAddress()), nil
			})
			if conn != nil {
				run("heartbeat", func() (string, error) {
					hctx, cancel := context.WithTimeout(ctx, a.timeout+time.Second)
					defer cancel()
					select {
					case <-time.After(1200 * time.Millisecond):
					case <-hctx.Done():
					}
					if conn.State() != knx.StateConnected {
						return "", fmt.Errorf("connection lost: %v", conn.Err())
					}
					return "connection state answered", nil
				})
				run("disconnect", func() (string, error) { return "", conn.Close() })
			}
			if err := a.emit("diagnose", res); err != nil {
				return err
			}
			if !res.OK {
				return errs.ErrFindings
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&nat, "nat", false, "Announce route back endpoints (through NAT)")
	return cmd
}
