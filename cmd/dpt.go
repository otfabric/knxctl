// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/otfabric/go-knx/dpt"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

type dptRecord struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Unit        string   `json:"unit,omitempty"`
	Bits        int      `json:"bits"`
	Kind        string   `json:"kind"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Enum        []string `json:"values,omitempty"`
}

func newDPTRecord(t *dpt.Type) dptRecord {
	r := dptRecord{ID: t.ID.String(), Name: t.Name, Description: t.Description, Unit: t.Unit, Bits: t.Bits, Kind: t.Kind.String(), Labels: t.Labels}
	if t.HasRange {
		mn, mx := t.Min, t.Max
		r.Min, r.Max = &mn, &mx
	}
	for i := 0; i < 256; i++ {
		if n, ok := t.Enum[uint8(i)]; ok {
			r.Enum = append(r.Enum, fmt.Sprintf("%d=%s", i, n))
		}
	}
	return r
}

type dptList struct {
	Types []dptRecord `json:"types"`
}

func (l dptList) Text() string {
	var sb strings.Builder
	for _, t := range l.Types {
		unit := ""
		if t.Unit != "" {
			unit = " [" + t.Unit + "]"
		}
		fmt.Fprintf(&sb, "%-9s %-36s %s%s\n", t.ID, t.Name, t.Description, unit)
	}
	return sb.String()
}

func (l dptList) Headers() []string { return []string{"ID", "NAME", "DESCRIPTION", "UNIT", "BITS"} }

func (l dptList) Rows() [][]string {
	rows := make([][]string, len(l.Types))
	for i, t := range l.Types {
		rows[i] = []string{t.ID, t.Name, t.Description, t.Unit, strconv.Itoa(t.Bits)}
	}
	return rows
}

type dptDescribe struct{ dptRecord }

func (d dptDescribe) Text() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n  %s\n  %d bits, %s values", d.ID, d.Name, d.Description, d.Bits, d.Kind)
	if d.Unit != "" {
		fmt.Fprintf(&sb, ", unit %s", d.Unit)
	}
	if d.Min != nil {
		fmt.Fprintf(&sb, "\n  range %v .. %v", *d.Min, *d.Max)
	}
	if len(d.Labels) == 2 {
		fmt.Fprintf(&sb, "\n  0 = %s, 1 = %s", d.Labels[0], d.Labels[1])
	}
	if len(d.Enum) > 0 {
		fmt.Fprintf(&sb, "\n  values: %s", strings.Join(d.Enum, ", "))
	}
	return sb.String()
}

type codecResult struct {
	DPT   string `json:"dpt"`
	Data  string `json:"data"`
	Small bool   `json:"small"`
	Value any    `json:"value"`
	Shown string `json:"text"`
}

func (c codecResult) Text() string {
	s := fmt.Sprintf("%s  %s  (DPT %s)", c.Data, c.Shown, c.DPT)
	if c.Small {
		s += ", sent in the APCI octet"
	}
	return s
}

func newDPTCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "dpt", Short: "Datapoint types: list, describe, encode, decode"}
	var main int
	list := &cobra.Command{
		Use:     "list",
		Short:   "List the datapoint types",
		Example: "  knxctl dpt list\n  knxctl dpt list --main 9",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			l := dptList{Types: []dptRecord{}}
			for _, t := range dpt.All() {
				if main == 0 || int(t.ID.Main) == main {
					l.Types = append(l.Types, newDPTRecord(t))
				}
			}
			return a.emit("dpt list", l)
		},
	}
	list.Flags().IntVar(&main, "main", 0, "Only the subtypes of this main number")
	describe := &cobra.Command{
		Use:     "describe <dpt>",
		Short:   "Describe a datapoint type",
		Example: "  knxctl dpt describe 9.001",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			t, err := lookupDPT(args[0])
			if err != nil {
				return err
			}
			return a.emit("dpt describe", dptDescribe{newDPTRecord(t)})
		},
	}
	encode := &cobra.Command{
		Use:     "encode <dpt> <value>",
		Short:   "Encode a value to the octets of a group telegram",
		Example: "  knxctl dpt encode 9.001 21.5\n  knxctl dpt encode 10.001 'Mon 12:30:00'\n  knxctl dpt encode 232.600 '#FF8000'",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			t, err := lookupDPT(args[0])
			if err != nil {
				return err
			}
			v, err := t.Parse(args[1])
			if err != nil {
				return errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "%v", err)
			}
			b, err := t.Encode(v)
			if err != nil {
				return errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "%v", err)
			}
			return a.emit("dpt encode", codecResult{DPT: t.ID.String(), Data: strings.ToUpper(hex.EncodeToString(b)), Small: t.Small(), Value: jsonValue(v), Shown: t.Format(v)})
		},
	}
	decode := &cobra.Command{
		Use:     "decode <dpt> <hex>",
		Short:   "Decode the octets of a group telegram",
		Example: "  knxctl dpt decode 9.001 0C33\n  knxctl dpt decode 1.001 01",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			t, err := lookupDPT(args[0])
			if err != nil {
				return err
			}
			b, err := hex.DecodeString(strings.ReplaceAll(strings.TrimPrefix(args[1], "0x"), " ", ""))
			if err != nil {
				return errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "%q: want hexadecimal octets", args[1])
			}
			v, err := t.Decode(b)
			if err != nil {
				return errs.New(errs.KindInvalidInput, errs.CodeInvalidValue, "%v", err)
			}
			return a.emit("dpt decode", codecResult{DPT: t.ID.String(), Data: strings.ToUpper(hex.EncodeToString(b)), Small: t.Small(), Value: jsonValue(v), Shown: t.Format(v)})
		},
	}
	cmd.AddCommand(list, describe, encode, decode)
	return cmd
}
