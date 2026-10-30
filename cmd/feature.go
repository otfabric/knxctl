// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/cobra"
)

// featureNames are the tunnelling v2 interface features.
var featureNames = map[string]knxnetip.FeatureID{
	"supported-emi":     knxnetip.FeatureSupportedEMIType,
	"device-descriptor": knxnetip.FeatureDeviceDescriptorType0,
	"bus-connection":    knxnetip.FeatureBusConnectionStatus,
	"manufacturer":      knxnetip.FeatureManufacturerCode,
	"active-emi":        knxnetip.FeatureActiveEMIType,
	"address":           knxnetip.FeatureIndividualAddress,
	"max-apdu":          knxnetip.FeatureMaxAPDULength,
	"info-service":      knxnetip.FeatureInfoServiceEnable,
}

func featureList() string {
	names := make([]string, 0, len(featureNames))
	for n := range featureNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func parseFeature(s string) (knxnetip.FeatureID, error) {
	if f, ok := featureNames[strings.ToLower(s)]; ok {
		return f, nil
	}
	v, err := strconv.ParseUint(s, 0, 8)
	if err != nil {
		return 0, errs.Invalid("feature %q: a number or %s", s, featureList())
	}
	return knxnetip.FeatureID(v), nil
}

type featureResult struct {
	Server  string `json:"server"`
	Feature string `json:"feature"`
	ID      uint8  `json:"id"`
	Value   string `json:"value"`
	Set     bool   `json:"set,omitempty"`
}

func (r featureResult) Text() string {
	if r.Set {
		return fmt.Sprintf("%s (0x%02X) set to %s", r.Feature, r.ID, r.Value)
	}
	return fmt.Sprintf("%s (0x%02X): %s", r.Feature, r.ID, r.Value)
}

func featureName(f knxnetip.FeatureID) string {
	for n, id := range featureNames {
		if id == f {
			return n
		}
	}
	return fmt.Sprintf("feature-%d", f)
}

func newTunnelFeatureCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "feature", Short: "Tunnelling v2 interface feature services (TUNNELLING_FEATURE_GET/_SET)"}
	var sec secureFlags
	run := func(c *cobra.Command, host string, set bool, f knxnetip.FeatureID, value []byte) error {
		// TCP unless --udp: feature services are tunnelling v2.
		opts := []tunnel.Option{tunnel.WithLogger(a.logger), tunnel.WithConnectTimeout(a.timeout)}
		cred, err := sec.credentials("")
		if err != nil {
			return err
		}
		switch {
		case cred != nil:
			opts = append(opts, tunnel.WithSecure(*cred))
			if sec.udp {
				opts = append(opts, tunnel.WithUDP())
			}
		case !sec.udp:
			opts = append(opts, tunnel.WithTCP())
		}
		conn, err := tunnel.Dial(c.Context(), host, opts...)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		r := featureResult{Server: host, Feature: featureName(f), ID: uint8(f)}
		if set {
			if err := conn.FeatureSet(c.Context(), f, value); err != nil {
				return err
			}
			r.Set, r.Value = true, strings.ToUpper(hex.EncodeToString(value))
			return a.emit("tunnel feature set", r)
		}
		v, err := conn.FeatureGet(c.Context(), f)
		if err != nil {
			return err
		}
		r.Value = strings.ToUpper(hex.EncodeToString(v))
		return a.emit("tunnel feature get", r)
	}
	get := &cobra.Command{
		Use:   "get host[:port] <feature>",
		Short: "Read an interface feature of a tunnelling v2 server",
		Long: "Open a tunnelling connection (TCP by default) and read a feature: " + featureList() +
			", or a feature number. Nothing is sent onto the KNX network.",
		Example: "  knxctl tunnel feature get 192.168.1.10 max-apdu\n  knxctl tunnel feature get 192.168.1.10 bus-connection --udp",
		Args:    cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := parseFeature(args[1])
			if err != nil {
				return err
			}
			return run(c, args[0], false, f, nil)
		},
	}
	set := &cobra.Command{
		Use:     "set host[:port] <feature> <hex value>",
		Short:   "Write an interface feature of a tunnelling v2 server (info-service)",
		Example: "  knxctl tunnel feature set 192.168.1.10 info-service 01",
		Args:    cobra.ExactArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := parseFeature(args[1])
			if err != nil {
				return err
			}
			v, err := hex.DecodeString(args[2])
			if err != nil || len(v) == 0 {
				return errs.Invalid("value %q: hexadecimal octets", args[2])
			}
			return run(c, args[0], true, f, v)
		},
	}
	for _, c := range []*cobra.Command{get, set} {
		sec.add(c.Flags()) // --udp: over UDP instead of TCP
		cmd.AddCommand(c)
	}
	return cmd
}
