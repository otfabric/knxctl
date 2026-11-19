// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/otfabric/go-knx/dpt"
)

// The bits of a bit set (DPT 21, 22, 26, 30) are named by dpt.Type.Flags,
// least significant bit first, "" for a bit without a name. knxctl shows
// the names of the set bits next to the value and takes names where it
// takes a value.

// dptFlag is a named bit of a datapoint type (dpt describe, dpt list).
type dptFlag struct {
	Bit  int    `json:"bit"`
	Name string `json:"name"`
}

// typeFlags returns the named bits of t, least significant first; nil for
// a type without named bits.
func typeFlags(t *dpt.Type) []dptFlag {
	var out []dptFlag
	for i, n := range t.Flags {
		if n != "" {
			out = append(out, dptFlag{Bit: i, Name: n})
		}
	}
	return out
}

// setFlags returns the names of the named bits that are set in v, a
// decoded value of t, least significant first; nil when t names no bits
// or none of them is set. Unnamed bits (reserved bits, the scene number of
// 26.001) are left out: the value shows them.
func setFlags(t *dpt.Type, v any) []string {
	u, ok := v.(uint64)
	if !ok || t.Kind != dpt.KindBits {
		return nil
	}
	var out []string
	for i, n := range t.Flags {
		if n != "" && u&(1<<uint(i)) != 0 {
			out = append(out, n)
		}
	}
	return out
}

// withFlags appends the names of the set bits to a formatted value.
func withFlags(text string, flags []string) string {
	if len(flags) == 0 {
		return text
	}
	return text + ": " + strings.Join(flags, ", ")
}

// parseDPTValue parses a value for t as t.Parse does and, for a bit set with
// named bits, also as a comma-separated list of bits (flagBit); "none" is
// no bit set.
func parseDPTValue(t *dpt.Type, s string) (any, error) {
	v, err := t.Parse(s)
	if err == nil || t.Kind != dpt.KindBits || len(typeFlags(t)) == 0 {
		return v, err
	}
	u, ferr := parseFlags(t, s)
	if ferr != nil {
		return nil, fmt.Errorf("%w; or bit names: %v", err, ferr)
	}
	return u, nil
}

// parseFlags parses a comma-separated list of bits of t.
func parseFlags(t *dpt.Type, s string) (uint64, error) {
	if strings.EqualFold(strings.TrimSpace(s), "none") {
		return 0, nil
	}
	var u uint64
	for _, part := range strings.Split(s, ",") {
		bit, err := flagBit(t, strings.TrimSpace(part))
		if err != nil {
			return 0, err
		}
		u |= 1 << uint(bit)
	}
	return u, nil
}

// flagBit returns the bit of t that s selects. Names match without regard
// to case, spaces and punctuation: first a whole name ("forceRequest",
// "force request"), then bitN (bit N, least significant 0), then a part of
// exactly one name ("out of service" for "corresponding Datapoint value is
// out of service").
func flagBit(t *dpt.Type, s string) (int, error) {
	k := flagKey(s)
	if k == "" {
		return 0, fmt.Errorf("an empty bit name")
	}
	var whole, part []int
	for i, n := range t.Flags {
		switch nk := flagKey(n); {
		case nk == "":
		case nk == k:
			whole = append(whole, i)
		case strings.Contains(nk, k):
			part = append(part, i)
		}
	}
	if len(whole) == 1 {
		return whole[0], nil
	}
	if len(whole) == 0 {
		if n, err := strconv.Atoi(strings.TrimPrefix(k, "bit")); err == nil && strings.HasPrefix(k, "bit") && strconv.Itoa(n) == k[3:] {
			if n >= t.Bits {
				return 0, fmt.Errorf("%q: %s has bits bit0..bit%d", s, t.ID, t.Bits-1)
			}
			return n, nil
		}
		if len(part) == 1 {
			return part[0], nil
		}
	}
	if m := append(whole, part...); len(m) > 1 {
		return 0, fmt.Errorf("%q names more than one bit of %s (%s): use a longer name or bitN", s, t.ID, bitList(m))
	}
	return 0, fmt.Errorf("%q is not a bit of %s (%s, or bit0..bit%d)", s, t.ID, flagList(t), t.Bits-1)
}

// bitList writes bit numbers as bitN.
func bitList(bits []int) string {
	s := make([]string, len(bits))
	for i, b := range bits {
		s[i] = "bit" + strconv.Itoa(b)
	}
	return strings.Join(s, ", ")
}

// flagKey is the form a bit name is matched in: lower case letters and
// digits only.
func flagKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// flagList lists the named bits of t for an error message.
func flagList(t *dpt.Type) string {
	var names []string
	for _, f := range typeFlags(t) {
		names = append(names, strconv.Quote(f.Name))
	}
	return strings.Join(names, ", ")
}
