// SPDX-License-Identifier: MIT

package cmd

import (
	"maps"
	"slices"

	"github.com/otfabric/go-knx/dpt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Shell completion: flags with a fixed set of values complete those
// values, file flags complete files of their type, and every other flag
// and positional argument completes nothing (no file names).

// fixed completes values and no files.
func fixed(values ...string) cobra.CompletionFunc {
	return cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp)
}

// files completes the files with these extensions (without the dot).
func files(ext ...string) cobra.CompletionFunc {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return ext, cobra.ShellCompDirectiveFilterFileExt
	}
}

// noFiles completes nothing: free values such as addresses and numbers.
func noFiles(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// dptIDs are the datapoint types of go-knx, with their names.
func dptIDs() []string {
	var ids []string
	for _, t := range dpt.All() {
		ids = append(ids, t.ID.String()+"\t"+t.Name)
	}
	return ids
}

func completeDPT(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return dptIDs(), cobra.ShellCompDirectiveNoFileComp
}

// positional completes the argument at index i with fn and nothing else.
func positional(at map[int]cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if fn, ok := at[len(args)]; ok {
			return fn(cmd, args, toComplete)
		}
		return noFiles(cmd, args, toComplete)
	}
}

// flagCompletions are the value completions of flags by name, whichever
// command defines them.
func flagCompletions() map[string]cobra.CompletionFunc {
	return map[string]cobra.CompletionFunc{
		"format":   fixed("text\thuman-readable", "json\tversioned JSON", "table\taligned columns"),
		"priority": fixed("low", "normal", "urgent", "system"),
		"service":  fixed(slices.Sorted(maps.Keys(familyNames))...),
		"dpt":      completeDPT,
		"keyring":  files("knxkeys"),
		"config":   files("json"),
	}
}

// argCompletions are the positional completions by command path.
func argCompletions() map[string]cobra.CompletionFunc {
	features := slices.Sorted(maps.Keys(featureNames))
	objects := slices.Sorted(maps.Keys(objectNames))
	return map[string]cobra.CompletionFunc{
		"knxctl dpt describe":       positional(map[int]cobra.CompletionFunc{0: completeDPT}),
		"knxctl dpt encode":         positional(map[int]cobra.CompletionFunc{0: completeDPT}),
		"knxctl dpt decode":         positional(map[int]cobra.CompletionFunc{0: completeDPT}),
		"knxctl tunnel feature get": positional(map[int]cobra.CompletionFunc{1: fixed(features...)}),
		"knxctl tunnel feature set": positional(map[int]cobra.CompletionFunc{1: fixed(features...)}),
		"knxctl property read":      positional(map[int]cobra.CompletionFunc{1: fixed(objects...)}),
		"knxctl property write":     positional(map[int]cobra.CompletionFunc{1: fixed(objects...)}),
		"knxctl keyring show":       positional(map[int]cobra.CompletionFunc{0: files("knxkeys")}),
	}
}

// registerCompletions gives every flag and positional argument of the tree
// a completion: the value sets above, else no file names.
func registerCompletions(root *cobra.Command) {
	byFlag, byCmd := flagCompletions(), argCompletions()
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "bool" || f.Value.Type() == "count" {
				return
			}
			if _, ok := c.GetFlagCompletionFunc(f.Name); ok {
				return
			}
			fn, ok := byFlag[f.Name]
			if !ok {
				fn = noFiles
			}
			_ = c.RegisterFlagCompletionFunc(f.Name, fn)
		})
		if fn, ok := byCmd[c.CommandPath()]; ok {
			c.ValidArgsFunction = fn
		} else if c.Runnable() && !c.HasSubCommands() && c.ValidArgsFunction == nil && len(c.ValidArgs) == 0 {
			c.ValidArgsFunction = noFiles
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
