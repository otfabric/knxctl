// SPDX-License-Identifier: MIT

// Package buildinfo resolves the build metadata knxctl reports with
// `knxctl version`: the version, tag, commit and build date injected at
// link time (main's -X flags, passed in with Set), else what the Go
// toolchain recorded in the binary.
package buildinfo

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
)

// Link-time metadata. main's -X flags (-X main.version=... etc.) reach
// these through Set; they can also be set directly with
// -X github.com/otfabric/knxctl/internal/buildinfo.version=...
var (
	version   string
	tag       string
	commit    string
	buildDate string
)

// readBuildInfo is debug.ReadBuildInfo; a test replaces it.
var readBuildInfo = debug.ReadBuildInfo

// pseudoVersion matches the time-and-commit suffix of a Go pseudo-version
// (v0.9.1-0.20261107212210-d178f634038c), which go build stamps into a
// binary built from a checkout.
var pseudoVersion = regexp.MustCompile(`[-.]\d{14}-[0-9a-f]{12}(\+incompatible)?$`)

// released reports a module version that names a release: not "(devel)",
// not a pseudo-version, not a build of a modified checkout.
func released(v string) bool {
	return v != "" && v != "(devel)" && !strings.Contains(v, "+dirty") && !pseudoVersion.MatchString(v)
}

// Set records the link-time metadata. Empty values, and the version
// "dev", mean not injected.
func Set(v, t, c, d string) {
	version, tag, commit, buildDate = v, t, c, d
}

// Info is the resolved build metadata. Every field is set: unknown values
// are "dev" (version) or "unknown"; Tag alone is empty when there is none.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Commit  string `json:"commit"`
	Built   string `json:"built"`
	// Libraries are the OT Fabric libraries (github.com/otfabric/go-*)
	// the binary is built with, sorted by name; never nil.
	Libraries []Library `json:"libraries"`
	Go        string    `json:"go"`
	Platform  string    `json:"platform"`
}

// Library is an OT Fabric library and its version (without the leading v).
type Library struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// libraries returns the github.com/otfabric/go-* dependencies of bi other
// than its main module, a replacement standing for the module it replaces.
func libraries(bi *debug.BuildInfo) []Library {
	libs := []Library{}
	for _, d := range bi.Deps {
		m := d
		if d.Replace != nil {
			m = d.Replace
		}
		if !strings.HasPrefix(d.Path, "github.com/otfabric/go-") || d.Path == bi.Main.Path {
			continue
		}
		v := m.Version
		if v == "" {
			v = d.Version
		}
		libs = append(libs, Library{Name: path.Base(d.Path), Version: strings.TrimPrefix(v, "v")})
	}
	slices.SortFunc(libs, func(a, b Library) int { return strings.Compare(a.Name, b.Name) })
	return libs
}

// Get resolves the build metadata of the program name.
func Get(name string) Info {
	i := Info{
		Name: name, Version: strings.TrimPrefix(version, "v"), Tag: tag, Commit: commit, Built: buildDate,
		Libraries: []Library{}, Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	if i.Version == "dev" {
		i.Version = ""
	}
	if bi, ok := readBuildInfo(); ok && bi != nil {
		i.Libraries = libraries(bi)
		// "go install github.com/otfabric/knxctl@vX.Y.Z" injects nothing;
		// the module version says it. go build of a checkout stamps a
		// pseudo-version: that is a development build.
		if i.Version == "" && released(bi.Main.Version) {
			i.Version = strings.TrimPrefix(bi.Main.Version, "v")
		}
		if i.Commit == "" {
			var rev string
			var modified bool
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					rev = s.Value
				case "vcs.modified":
					modified = s.Value == "true"
				}
			}
			if rev != "" {
				i.Commit = rev
				if modified {
					i.Commit += "-dirty"
				}
			}
		}
	}
	if i.Version == "" {
		i.Version = "dev"
	}
	if i.Commit == "" {
		i.Commit = "unknown"
	}
	if i.Built == "" {
		i.Built = "unknown"
	}
	return i
}

// Text is the one-line form:
// name version (tag t, commit c, built d, lib v, ..., go, goos/goarch); the
// tag part is left out when there is no tag, the libraries when there are
// none.
func (i Info) Text() string {
	var b strings.Builder
	b.WriteString(i.Name + " " + i.Version + " (")
	if i.Tag != "" {
		b.WriteString("tag " + i.Tag + ", ")
	}
	b.WriteString("commit " + i.Commit + ", built " + i.Built + ", ")
	for _, l := range i.Libraries {
		b.WriteString(l.Name + " " + l.Version + ", ")
	}
	b.WriteString(i.Go + ", " + i.Platform + ")")
	return b.String()
}

// JSON is the one-line JSON form, keys in the order name, version, tag,
// commit, built, libraries, go, platform.
func (i Info) JSON() string {
	if i.Libraries == nil {
		i.Libraries = []Library{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(i) // a struct of strings always encodes
	return strings.TrimSuffix(buf.String(), "\n")
}
