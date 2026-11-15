// SPDX-License-Identifier: MIT

package cmd

import (
	"strconv"

	"github.com/spf13/pflag"
)

// intValue is an int flag that holds exactly the value given or refuses
// it. pflag's IntVar parses 64 bits and converts the result to int, which
// on a 32-bit platform (the release builds linux/armv7) keeps only the
// low 32 bits of a larger value; this one parses with the size of the
// platform's int, so such a value is a flag error (exit 2).
type intValue struct{ p *int }

func (v intValue) Set(s string) error {
	n, err := strconv.ParseInt(s, 0, strconv.IntSize)
	if err != nil {
		return err
	}
	*v.p = int(n)
	return nil
}

// Type is pflag's, so that the usage text stays the same.
func (v intValue) Type() string { return "int" }

func (v intValue) String() string { return strconv.Itoa(*v.p) }

// intVarP registers an int flag of intValue; shorthand may be "".
func intVarP(f *pflag.FlagSet, p *int, name, shorthand string, value int, usage string) {
	*p = value
	f.VarP(intValue{p}, name, shorthand, usage)
}
