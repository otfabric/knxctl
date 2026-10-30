// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/client"
	"github.com/otfabric/go-knx/datasecure"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/pflag"
)

// remoteFlags select remote device management: a device on the KNX
// network, reached through a tunnel, instead of the KNXnet/IP server's
// local device management.
type remoteFlags struct {
	device    string
	connected bool
	toolKey   string
}

func (r *remoteFlags) add(f *pflag.FlagSet) {
	f.StringVar(&r.device, "device", "", "Individual address of a device on the KNX network: remote device management (A_PropertyValue_*) through a tunnel instead of the server's local device management")
	f.BoolVar(&r.connected, "connected", false, "With --device: access the device in a transport connection (default connectionless)")
	f.StringVar(&r.toolKey, "tool-key", envOr("KNXCTL_TOOL_KEY", ""), "With --device: the device's tool key, 32 hex digits, to secure the access with KNX Data Secure (default: the device's tool key in --keyring, if any) (env KNXCTL_TOOL_KEY)")
}

// remote is an open remote management session.
type remote struct {
	c       *client.Client
	d       *client.Device
	addr    knx.IndividualAddress
	secured bool
}

func (r *remote) close() {
	_ = r.d.Close()
	_ = r.c.Close()
}

// dialRemote opens a tunnel to host and the remote management of the
// device. Its tool key (--tool-key, else the keyring's) secures it.
func dialRemote(ctx context.Context, a *app, host string, tcp, nat bool, sec *secureFlags, rf *remoteFlags) (*remote, error) {
	dev, err := knx.ParseIndividualAddress(rf.device)
	if err != nil {
		return nil, errs.Invalid("--device: %v", err)
	}
	var dopts []client.DeviceOption
	if rf.connected {
		dopts = append(dopts, client.Connected())
	}
	switch {
	case rf.toolKey != "":
		b, err := hex.DecodeString(rf.toolKey)
		var k datasecure.Key
		if err != nil || len(b) != len(k) {
			return nil, errs.Invalid("--tool-key: want 32 hex digits")
		}
		copy(k[:], b)
		dopts = append(dopts, client.WithToolKey(k))
	default:
		kr, err := sec.keyring()
		if err != nil {
			return nil, err
		}
		if kr != nil {
			if d := kr.Device(dev); d != nil && d.HasToolKey {
				dopts = append(dopts, client.WithToolKey(datasecure.Key(d.ToolKey)))
			}
		}
	}
	topts := []tunnel.Option{tunnel.WithLogger(a.logger), tunnel.WithConnectTimeout(a.timeout), tunnel.WithNAT(nat)}
	if tcp {
		topts = append(topts, tunnel.WithTCP())
	}
	if topts, err = sec.tunnelOptions(topts, ""); err != nil {
		return nil, err
	}
	c, err := client.DialTunnel(ctx, host, topts, client.WithLogger(a.logger))
	if err != nil {
		return nil, err
	}
	d := c.Device(dev, dopts...)
	return &remote{c: c, d: d, addr: dev, secured: d.Secured()}, nil
}

// objectIndex finds the index of an interface object type in the device's
// interface object list (device object, PID 71).
func (r *remote) objectIndex(ctx context.Context, objType uint16) (uint8, error) {
	if objType == 0 {
		return 0, nil
	}
	n, err := r.d.PropertyElements(ctx, 0, 71)
	if err != nil {
		return 0, err
	}
	for start := 1; start <= n; start += 15 {
		b, err := r.d.PropertyRead(ctx, 0, 71, uint16(start), uint8(min(15, n-start+1)))
		if err != nil {
			return 0, err
		}
		for i := 0; i+1 < len(b); i += 2 {
			if uint16(b[i])<<8|uint16(b[i+1]) == objType {
				return uint8(start - 1 + i/2), nil
			}
		}
	}
	return 0, errs.New(errs.KindRejected, errs.CodeRejected, "%s has no interface object of type 0x%04X", r.addr, objType)
}

// remoteContext bounds a remote access by the global timeout.
func remoteContext(ctx context.Context, a *app) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, a.timeout+5*time.Second)
}
