// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/cemi"
	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

type decodedFrame struct {
	Layer   string         `json:"layer"`
	Service string         `json:"service"`
	Length  int            `json:"length"`
	Fields  map[string]any `json:"fields,omitempty"`
	CEMI    *decodedCEMI   `json:"cemi,omitempty"`
}

type decodedCEMI struct {
	Code        string `json:"messageCode"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Priority    string `json:"priority,omitempty"`
	HopCount    int    `json:"hopCount,omitempty"`
	Standard    bool   `json:"standardFrame,omitempty"`
	ConfirmErr  bool   `json:"confirmError,omitempty"`
	TPCI        string `json:"tpci,omitempty"`
	APCI        string `json:"apci,omitempty"`
	Data        string `json:"data,omitempty"`
	Raw         string `json:"raw"`
}

func (d decodedFrame) Text() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s (%d octets)", d.Layer, d.Service, d.Length)
	for k, v := range d.Fields {
		fmt.Fprintf(&sb, "\n  %s: %v", k, v)
	}
	if c := d.CEMI; c != nil {
		fmt.Fprintf(&sb, "\n  cEMI %s", c.Code)
		if c.Destination != "" {
			fmt.Fprintf(&sb, " %s -> %s, %s, hop count %d", c.Source, c.Destination, c.Priority, c.HopCount)
		}
		if c.APCI != "" {
			fmt.Fprintf(&sb, "\n  %s %s %s", c.TPCI, c.APCI, c.Data)
		}
	}
	return sb.String()
}

func decodeCEMI(b []byte) (*decodedCEMI, error) {
	f, err := cemi.Parse(b)
	if err != nil {
		return nil, err
	}
	d := &decodedCEMI{Code: f.Code().String(), Raw: strings.ToUpper(hex.EncodeToString(b))}
	if ld, ok := f.(*cemi.LData); ok {
		d.Source, d.Destination, d.Priority, d.HopCount = ld.Source.String(), ld.DestinationString(), ld.Priority().String(), int(ld.HopCount())
		d.Standard, d.ConfirmErr = ld.Standard(), ld.ConfirmError()
		if p, err := apdu.Parse(ld.TPDU); err == nil {
			d.TPCI = p.TPCI.Kind.String()
			if !p.TPCI.IsControl() {
				d.APCI = p.APCI.String()
				data := p.Data
				if p.APCI.Has6BitData() && len(data) == 0 {
					data = []byte{p.Short}
				}
				d.Data = strings.ToUpper(hex.EncodeToString(data))
			}
		}
	}
	return d, nil
}

// decodeFrame decodes a KNXnet/IP frame and the cEMI frame it carries. The
// octets are whatever the user pasted or a capture held: nothing about them
// is trusted.
func decodeFrame(b []byte) (decodedFrame, error) {
	m, err := knxnetip.Parse(b)
	if err != nil {
		return decodedFrame{}, err
	}
	d := decodedFrame{Layer: "KNXnet/IP", Service: m.Service().String(), Length: len(b), Fields: map[string]any{}}
	var payload []byte
	switch m := m.(type) {
	case *knxnetip.TunnellingRequest:
		d.Fields["channel"], d.Fields["sequence"] = m.Channel, m.Seq
		payload = m.CEMI
	case *knxnetip.TunnellingAck:
		d.Fields["channel"], d.Fields["sequence"], d.Fields["status"] = m.Channel, m.Seq, m.Status.String()
	case *knxnetip.RoutingIndication:
		payload = m.CEMI
	case *knxnetip.DeviceConfigurationRequest:
		d.Fields["channel"], d.Fields["sequence"] = m.Channel, m.Seq
		payload = m.CEMI
	case *knxnetip.ConnectRequest:
		d.Fields["control"], d.Fields["data"], d.Fields["connectionType"], d.Fields["layer"] = m.Control.String(), m.Data.String(), m.CRI.Type.String(), m.CRI.Layer.String()
	case *knxnetip.ConnectResponse:
		d.Fields["channel"], d.Fields["status"] = m.Channel, m.Status.String()
		if m.Status == knxnetip.StatusNoError {
			d.Fields["data"], d.Fields["address"] = m.Data.String(), m.CRD.Address.String()
		}
	case *knxnetip.ConnectionStateRequest:
		d.Fields["channel"], d.Fields["control"] = m.Channel, m.Control.String()
	case *knxnetip.ConnectionStateResponse:
		d.Fields["channel"], d.Fields["status"] = m.Channel, m.Status.String()
	case *knxnetip.DisconnectRequest:
		d.Fields["channel"], d.Fields["control"] = m.Channel, m.Control.String()
	case *knxnetip.DisconnectResponse:
		d.Fields["channel"], d.Fields["status"] = m.Channel, m.Status.String()
	case *knxnetip.SearchRequest:
		d.Fields["discovery"] = m.Discovery.String()
	case *knxnetip.DescriptionRequest:
		d.Fields["control"] = m.Control.String()
	case *knxnetip.SearchResponse:
		d.Fields["control"] = m.Control.String()
		d.Fields["device"] = newDeviceRecord(m.Control.Addr.String(), m.DIBs)
	case *knxnetip.DescriptionResponse:
		d.Fields["device"] = newDeviceRecord("", m.DIBs)
	case *knxnetip.Unknown:
		d.Fields["body"] = strings.ToUpper(hex.EncodeToString(m.Body))
	}
	if payload != nil {
		c, err := decodeCEMI(payload)
		if err != nil {
			d.Fields["cemiError"] = err.Error()
		}
		d.CEMI = c
	}
	if len(d.Fields) == 0 {
		d.Fields = nil
	}
	return d, nil
}

func newDecodeCmd(a *app) *cobra.Command {
	var asCEMI bool
	cmd := &cobra.Command{
		Use:   "decode <hex>",
		Short: "Decode a KNXnet/IP frame or a cEMI frame given in hexadecimal",
		Example: `  knxctl decode 061004200015040100001100bce0110a0a03010081
  knxctl decode --cemi 2900bce0110a0a03010081`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			b, err := hex.DecodeString(strings.ReplaceAll(strings.TrimPrefix(args[0], "0x"), " ", ""))
			if err != nil {
				return errs.Invalid("%q: want hexadecimal octets", args[0])
			}
			if asCEMI {
				c, err := decodeCEMI(b)
				if err != nil {
					return err
				}
				return a.emit("decode", decodedFrame{Layer: "cEMI", Service: c.Code, Length: len(b), CEMI: c})
			}
			d, err := decodeFrame(b)
			if err != nil {
				return err
			}
			return a.emit("decode", d)
		},
	}
	cmd.Flags().BoolVar(&asCEMI, "cemi", false, "The octets are a cEMI frame, not a KNXnet/IP frame")
	return cmd
}
