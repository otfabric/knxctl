// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/otfabric/go-knx/keyring"
	"github.com/spf13/cobra"
)

// keyringResult summarises a keyring. Passwords and keys are included
// only with --secrets.
type keyringResult struct {
	Project    string           `json:"project"`
	CreatedBy  string           `json:"createdBy"`
	Created    string           `json:"created"`
	Backbone   *keyringBackbone `json:"backbone,omitempty"`
	Interfaces []keyringIface   `json:"interfaces"`
	Groups     []keyringGroup   `json:"groups"`
	Devices    []keyringDevice  `json:"devices"`
	Secrets    bool             `json:"secretsShown"`
}

type keyringBackbone struct {
	MulticastAddress string `json:"multicastAddress"`
	LatencyMs        int64  `json:"latencyMs"`
	Key              string `json:"key,omitempty"`
}

type keyringIface struct {
	Address        string              `json:"address"`
	Type           string              `json:"type"`
	Host           string              `json:"host,omitempty"`
	UserID         uint8               `json:"userId,omitempty"`
	Password       string              `json:"password,omitempty"`
	Authentication string              `json:"authentication,omitempty"`
	Groups         map[string][]string `json:"groups,omitempty"`
}

type keyringGroup struct {
	Address string `json:"address"`
	Key     string `json:"key,omitempty"`
}

type keyringDevice struct {
	Address            string `json:"address"`
	SequenceNumber     uint64 `json:"sequenceNumber"`
	ToolKey            string `json:"toolKey,omitempty"`
	ManagementPassword string `json:"managementPassword,omitempty"`
	Authentication     string `json:"authentication,omitempty"`
}

func (r keyringResult) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "project %q, created %s by %s\n", r.Project, r.Created, r.CreatedBy)
	if bb := r.Backbone; bb != nil {
		fmt.Fprintf(&b, "backbone %s latency %d ms%s\n", bb.MulticastAddress, bb.LatencyMs, opt(" key ", bb.Key))
	}
	for _, i := range r.Interfaces {
		fmt.Fprintf(&b, "interface %s %s", i.Address, i.Type)
		if i.Host != "" {
			fmt.Fprintf(&b, " host %s", i.Host)
		}
		if i.UserID != 0 {
			fmt.Fprintf(&b, " user %d", i.UserID)
		}
		b.WriteString(opt(" password ", i.Password) + opt(" authentication ", i.Authentication))
		gs := make([]string, 0, len(i.Groups))
		for g, s := range i.Groups {
			gs = append(gs, g+" <- "+strings.Join(s, ","))
		}
		sort.Strings(gs)
		for _, g := range gs {
			b.WriteString("\n  group " + g)
		}
		b.WriteString("\n")
	}
	for _, g := range r.Groups {
		fmt.Fprintf(&b, "secure group %s%s\n", g.Address, opt(" key ", g.Key))
	}
	for _, d := range r.Devices {
		fmt.Fprintf(&b, "device %s sequence %d%s%s%s\n", d.Address, d.SequenceNumber, opt(" tool key ", d.ToolKey), opt(" management ", d.ManagementPassword), opt(" authentication ", d.Authentication))
	}
	if !r.Secrets {
		b.WriteString("(keys and passwords hidden; --secrets shows them)\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func opt(label, v string) string {
	if v == "" {
		return ""
	}
	return label + v
}

func newKeyringCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "keyring", Short: "Inspect KNX keyrings (.knxkeys)"}
	var password string
	var secrets bool
	show := &cobra.Command{
		Use:   "show file.knxkeys",
		Short: "Verify, decrypt and summarise a keyring",
		Long: `Verify the signature of an ETS keyring, decrypt it and list its backbone,
tunnelling interfaces and users, Data Secure groups and devices. Keys and
passwords are shown only with --secrets.`,
		Example: "  KNXCTL_KEYRING_PASSWORD=... knxctl keyring show project.knxkeys",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			kr, err := (&secureFlags{keyringPath: args[0], keyringPassword: password}).keyring()
			if err != nil {
				return err
			}
			return a.emit("keyring show", summarise(kr, secrets))
		},
	}
	show.Flags().StringVar(&password, "keyring-password", envOr("KNXCTL_KEYRING_PASSWORD", ""), "Password of the keyring (env KNXCTL_KEYRING_PASSWORD)")
	show.Flags().BoolVar(&secrets, "secrets", false, "Show keys and passwords")
	cmd.AddCommand(show)
	return cmd
}

func summarise(kr *keyring.Keyring, secrets bool) keyringResult {
	secret := func(s string) string {
		if secrets {
			return s
		}
		return ""
	}
	key := func(k keyring.Key) string { return secret(hex.EncodeToString(k[:])) }
	r := keyringResult{Project: kr.Project, CreatedBy: kr.CreatedBy, Created: kr.Created, Secrets: secrets,
		Interfaces: []keyringIface{}, Groups: []keyringGroup{}, Devices: []keyringDevice{}}
	if bb := kr.Backbone; bb != nil {
		r.Backbone = &keyringBackbone{MulticastAddress: bb.MulticastAddress, LatencyMs: bb.Latency.Milliseconds(), Key: key(bb.Key)}
	}
	for _, i := range kr.Interfaces {
		ki := keyringIface{Address: i.IndividualAddress.String(), Type: string(i.Type), UserID: i.UserID, Password: secret(i.Password), Authentication: secret(i.Authentication)}
		if i.HasHost {
			ki.Host = i.Host.String()
		}
		if len(i.Groups) > 0 {
			ki.Groups = map[string][]string{}
			for g, ss := range i.Groups {
				var s []string
				for _, a := range ss {
					s = append(s, a.String())
				}
				ki.Groups[g.String()] = s
			}
		}
		r.Interfaces = append(r.Interfaces, ki)
	}
	for g, k := range kr.GroupKeys {
		r.Groups = append(r.Groups, keyringGroup{Address: g.String(), Key: key(k)})
	}
	sort.Slice(r.Groups, func(i, j int) bool { return r.Groups[i].Address < r.Groups[j].Address })
	for _, d := range kr.Devices {
		kd := keyringDevice{Address: d.IndividualAddress.String(), SequenceNumber: d.SequenceNumber, ManagementPassword: secret(d.ManagementPassword), Authentication: secret(d.Authentication)}
		if d.HasToolKey {
			kd.ToolKey = key(d.ToolKey)
		}
		r.Devices = append(r.Devices, kd)
	}
	return r
}
