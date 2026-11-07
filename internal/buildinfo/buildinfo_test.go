// SPDX-License-Identifier: MIT

package buildinfo

import (
	"encoding/json"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

const sha = "1802d0f9afe8642d1e278120b76fcdfac967cb9e"

var goPlatform = runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH

// with sets the link-time metadata and the recorded build information for
// one test.
func with(t *testing.T, v, tg, c, d string, bi *debug.BuildInfo) {
	t.Helper()
	oldRead := readBuildInfo
	t.Cleanup(func() { Set("", "", "", ""); readBuildInfo = oldRead })
	Set(v, tg, c, d)
	readBuildInfo = func() (*debug.BuildInfo, bool) { return bi, bi != nil }
}

func TestTextAllFields(t *testing.T) {
	with(t, "3.5.1", "v3.5.1", sha, "2026-11-05T15:41:49Z", nil)
	want := "knxctl 3.5.1 (tag v3.5.1, commit " + sha + ", built 2026-11-05T15:41:49Z, " + goPlatform + ")"
	if got := Get("knxctl").Text(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestTextWithoutTag(t *testing.T) {
	with(t, "v3.5.1-2-gabcdef", "", sha, "2026-11-05T15:41:49Z", nil)
	want := "knxctl 3.5.1-2-gabcdef (commit " + sha + ", built 2026-11-05T15:41:49Z, " + goPlatform + ")"
	if got := Get("knxctl").Text(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDevFallback(t *testing.T) {
	for _, v := range []string{"", "dev"} {
		with(t, v, "", "", "", nil)
		want := "knxctl dev (commit unknown, built unknown, " + goPlatform + ")"
		if got := Get("knxctl").Text(); got != want {
			t.Fatalf("version %q: got %q, want %q", v, got, want)
		}
	}
	// A development build of a checkout: the toolchain's VCS stamp.
	with(t, "", "", "", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "true"}}})
	want := "knxctl dev (commit " + sha + "-dirty, built unknown, " + goPlatform + ")"
	if got := Get("knxctl").Text(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	with(t, "", "", "", "", &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "false"}}})
	if got := Get("knxctl").Commit; got != sha {
		t.Fatalf("clean checkout commit %q", got)
	}
}

// go install …@vX.Y.Z injects nothing: the module version is reported.
func TestGoInstallVersion(t *testing.T) {
	with(t, "", "", "", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.10.0"}})
	if i := Get("knxctl"); i.Version != "0.10.0" || i.Tag != "" || i.Commit != "unknown" {
		t.Fatalf("%+v", i)
	}
	// go build of a checkout stamps a pseudo-version: a development build.
	for _, v := range []string{"v0.9.1-0.20261107212210-d178f634038c+dirty", "v0.9.1-0.20261107212210-d178f634038c", "v0.0.0-20261107212210-d178f634038c", "v1.2.3+dirty"} {
		with(t, "", "", "", "", &debug.BuildInfo{Main: debug.Module{Version: v}})
		if i := Get("knxctl"); i.Version != "dev" {
			t.Fatalf("%s: %+v", v, i)
		}
	}
	// An injected version wins over the module version.
	with(t, "0.9.0", "", "", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.10.0"}})
	if i := Get("knxctl"); i.Version != "0.9.0" {
		t.Fatalf("%+v", i)
	}
}

func TestJSONKeyOrderAndValues(t *testing.T) {
	with(t, "3.5.1", "v3.5.1", sha, "2026-11-05T15:41:49Z", nil)
	want := `{"name":"knxctl","version":"3.5.1","tag":"v3.5.1","commit":"` + sha + `","built":"2026-11-05T15:41:49Z","go":"` +
		runtime.Version() + `","platform":"` + runtime.GOOS + "/" + runtime.GOARCH + `"}`
	if got := Get("knxctl").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	with(t, "", "", "", "", nil)
	got := Get("knxctl").JSON()
	if strings.Contains(got, "\n") {
		t.Fatalf("not one line: %q", got)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 7 || m["version"] != "dev" || m["tag"] != "" || m["commit"] != "unknown" || m["built"] != "unknown" {
		t.Fatalf("dev JSON %s", got)
	}
}
