// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/datasecure"
	"github.com/otfabric/go-knx/keyring"
	"github.com/otfabric/go-knx/secure"
	"github.com/otfabric/go-knx/tunnel"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/spf13/pflag"
)

// secureFlags select KNX IP Secure and KNX Data Secure: a keyring, or the
// passwords and keys given directly. Secrets are read from the
// environment by default so that they stay out of shell histories and
// process lists; knxctl never prints them.
type secureFlags struct {
	keyringPath     string
	keyringPassword string
	userID          int
	userPassword    string
	devicePassword  string
	backboneKey     string

	kr *keyring.Keyring
}

func (s *secureFlags) add(f *pflag.FlagSet) {
	f.StringVar(&s.keyringPath, "keyring", envOr("KNXCTL_KEYRING", ""), "ETS keyring (.knxkeys) with the secure tunnelling users, Data Secure group keys and backbone key (env KNXCTL_KEYRING)")
	f.StringVar(&s.keyringPassword, "keyring-password", envOr("KNXCTL_KEYRING_PASSWORD", ""), "Password of the keyring (env KNXCTL_KEYRING_PASSWORD)")
	f.IntVar(&s.userID, "user-id", 0, "KNX IP Secure tunnelling user (2..127; with --keyring it selects the user's interface)")
	f.StringVar(&s.userPassword, "user-password", envOr("KNXCTL_USER_PASSWORD", ""), "Password of the secure tunnelling user (env KNXCTL_USER_PASSWORD)")
	f.StringVar(&s.devicePassword, "device-password", envOr("KNXCTL_DEVICE_PASSWORD", ""), "Device authentication password of the secure server, to verify it (env KNXCTL_DEVICE_PASSWORD)")
	f.StringVar(&s.backboneKey, "backbone-key", envOr("KNXCTL_BACKBONE_KEY", ""), "KNX IP Secure routing backbone key, 32 hex digits (env KNXCTL_BACKBONE_KEY)")
}

// keyring loads the keyring once; nil without --keyring.
func (s *secureFlags) keyring() (*keyring.Keyring, error) {
	if s.keyringPath == "" || s.kr != nil {
		return s.kr, nil
	}
	kr, err := keyring.LoadFile(s.keyringPath, s.keyringPassword)
	if err != nil {
		return nil, errs.New(errs.KindInvalidInput, errs.CodeInvalidInput, "--keyring: %v", err)
	}
	s.kr = kr
	return kr, nil
}

// credentials returns the secure tunnelling credentials, nil for a plain
// tunnel. With a keyring the interface is the one of --tunnel-address,
// else of --user-id, else the first tunnelling interface with a user.
func (s *secureFlags) credentials(tunnelIA string) (*secure.Credentials, error) {
	if s.userID < 0 || s.userID > 127 {
		return nil, errs.Invalid("--user-id %d (2..127)", s.userID)
	}
	kr, err := s.keyring()
	if err != nil {
		return nil, err
	}
	if kr != nil && s.userPassword == "" {
		var itf *keyring.Interface
		switch {
		case tunnelIA != "":
			ia, err := knx.ParseIndividualAddress(tunnelIA)
			if err != nil {
				return nil, errs.Invalid("--tunnel-address: %v", err)
			}
			if itf = kr.Interface(ia); itf == nil || itf.UserID == 0 {
				return nil, errs.Invalid("--tunnel-address %s: no secure tunnelling interface in the keyring", ia)
			}
		default:
			for i := range kr.Interfaces {
				x := &kr.Interfaces[i]
				if x.Type == keyring.Tunneling && x.UserID != 0 && (s.userID == 0 || int(x.UserID) == s.userID) {
					itf = x
					break
				}
			}
			if itf == nil && s.userID != 0 {
				return nil, errs.Invalid("--user-id %d: not in the keyring", s.userID)
			}
		}
		if itf == nil {
			return nil, nil // a keyring for Data Secure or routing only
		}
		c, err := itf.Credentials()
		if err != nil {
			return nil, errs.Invalid("keyring interface %s: %v", itf.IndividualAddress, err)
		}
		return &c, nil
	}
	if s.userID == 0 && s.userPassword == "" {
		return nil, nil
	}
	if s.userID == 0 || s.userPassword == "" {
		return nil, errs.Invalid("secure tunnelling needs --user-id and --user-password (or --keyring)")
	}
	c, err := secure.NewCredentials(uint8(s.userID), s.userPassword, s.devicePassword)
	if err != nil {
		return nil, errs.Invalid("secure credentials: %v", err)
	}
	return &c, nil
}

// dataSecure returns the Data Secure tables of the keyring, nil when it
// has no group keys.
func (s *secureFlags) dataSecure() (*datasecure.Table, error) {
	kr, err := s.keyring()
	if err != nil || kr == nil || len(kr.GroupKeys) == 0 {
		return nil, err
	}
	t, err := kr.DataSecure(0)
	if err != nil {
		return nil, errs.Invalid("keyring: %v", err)
	}
	return t, nil
}

// backbone returns the secure routing key: --backbone-key, else the
// keyring's; ok is false for plain routing.
func (s *secureFlags) backbone() (key secure.Key, latency time.Duration, ok bool, err error) {
	if s.backboneKey != "" {
		b, err := hex.DecodeString(s.backboneKey)
		if err != nil || len(b) != len(key) {
			return key, 0, false, errs.Invalid("--backbone-key: want 32 hex digits")
		}
		copy(key[:], b)
		return key, 0, true, nil
	}
	kr, err := s.keyring()
	if err != nil || kr == nil {
		return key, 0, false, err
	}
	key, latency, ok = kr.BackboneKey()
	return key, latency, ok, nil
}

// tunnelOptions adds the secure session of the credentials, if any.
func (s *secureFlags) tunnelOptions(opts []tunnel.Option, tunnelIA string) ([]tunnel.Option, error) {
	cred, err := s.credentials(tunnelIA)
	if err != nil {
		return nil, err
	}
	if cred != nil {
		opts = append(opts, tunnel.WithSecure(*cred))
	}
	return opts, nil
}
