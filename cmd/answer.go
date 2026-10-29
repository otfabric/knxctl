// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/dpt"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

type answerResult struct {
	Group    string `json:"group"`
	Answered int    `json:"answered"`
}

func (r answerResult) Text() string { return fmt.Sprintf("answered %d reads of %s", r.Answered, r.Group) }

func newGroupAnswerCmd(a *app) *cobra.Command {
	var lf linkFlags
	var dptID, raw string
	var small bool
	var count int
	var duration time.Duration
	cmd := &cobra.Command{
		Use:   "answer <group> [value]",
		Short: "Answer A_GroupValue_Read of a group with a value, like a device with the R flag (transmits)",
		Long: `Wait for A_GroupValue_Read telegrams to the group and answer each with
A_GroupValue_Response of the value, until --count answers, --duration or
interruption. This makes knxctl behave as a device with the R flag on that
group: use it on test installations and simulators.`,
		Example: "  knxctl group answer 1/2/4 21.5 --dpt 9.001 --gateway 192.168.1.10 --count 1",
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
			if len(args) == 2 {
				value = args[1]
			} else if raw == "" {
				return errs.New(errs.KindUsage, errs.CodeInvalidInput, "a value or --raw is required")
			}
			gv, err := parseValue(ty, raw, small, value)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
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
			sub := c.Subscribe(64, g)
			a.warn("answering reads of %s via %s", g, via(&lf))
			res := answerResult{Group: g.String()}
			for count == 0 || res.Answered < count {
				select {
				case <-ctx.Done():
					return a.emit("group answer", res)
				case t, ok := <-sub.C():
					if !ok {
						return c.Err()
					}
					if t.Service != apdu.GroupValueReadService {
						continue
					}
					sctx, scancel := context.WithTimeout(ctx, a.timeout)
					err := c.GroupRespond(sctx, g, gv)
					scancel()
					if err != nil {
						return err
					}
					res.Answered++
				}
			}
			return a.emit("group answer", res)
		},
	}
	lf.add(cmd)
	cmd.Flags().StringVarP(&dptID, "dpt", "t", "", "Datapoint type of the value")
	cmd.Flags().StringVar(&raw, "raw", "", "Answer with these octets (hexadecimal)")
	cmd.Flags().BoolVar(&small, "small", false, "With --raw: one octet 00..3F in the APCI")
	cmd.Flags().IntVarP(&count, "count", "n", 0, "Stop after this many answers")
	cmd.Flags().DurationVar(&duration, "duration", 0, "Stop after this time")
	return cmd
}
