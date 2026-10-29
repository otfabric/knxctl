// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"
	"time"
)

// routingArgs selects a private multicast group on the loopback
// interface where it supports multicast.
func routingArgs(t *testing.T) []string {
	t.Helper()
	requireMulticast(t)
	args := []string{"--multicast", testGroup(t).String()}
	if lo := loopbackInterface(); lo != "" {
		args = append(args, "--interface", lo)
	}
	return args
}

// routedWrites sends group writes over routing until the command ends.
func routedWrites(t *testing.T, b *background, link []string, args ...string) result {
	t.Helper()
	for !b.ended() {
		r := run(t, append(append([]string{"group", "write", "--routing", "--source", "1.1.250"}, link...), args...)...)
		want(t, r, 0, "sent via routing")
		time.Sleep(10 * time.Millisecond)
	}
	return b.wait(t)
}

func TestRoutingMonitor(t *testing.T) {
	link := routingArgs(t)
	b := start(t, append([]string{"routing", "monitor", "--count", "2", "--map", "1/2/3=1.001", "--format", "json"}, link...)...)
	b.waitErr(t, "listening via routing")
	r := routedWrites(t, b, link, "1/2/3", "on")
	want(t, r, 0)
	for _, rec := range jsonLines(t, r.out) {
		if rec["source"] != "1.1.250" || rec["group"] != "1/2/3" || rec["value"] != true || rec["service"] != "write" {
			t.Fatalf("record %v", rec)
		}
	}
}

func TestRoutingGroupListenAndMonitor(t *testing.T) {
	link := routingArgs(t)
	b := start(t, append([]string{"group", "listen", "1/2/4", "--routing", "-n", "1", "--dpt", "9.001"}, link...)...)
	b.waitErr(t, "listening via routing")
	want(t, routedWrites(t, b, link, "--dpt", "9.001", "1/2/4", "21.5"), 0, "1.1.250", "-> 1/2/4", "(21.5 °C)")

	b = start(t, append([]string{"monitor", "--routing", "-n", "1"}, link...)...)
	b.waitErr(t, "listening via routing")
	want(t, routedWrites(t, b, link, "1/2/5", "--raw", "0D", "--small"), 0, "-> 1/2/5", " 0D")
}

func TestRoutingRespondAndRead(t *testing.T) {
	link := routingArgs(t)
	// An answering knxctl on routing, read over routing.
	b := start(t, append([]string{"group", "answer", "1/2/7", "on", "--routing", "--source", "1.1.251", "-n", "1"}, link...)...)
	b.waitErr(t, "answering reads of 1/2/7")
	r := run(t, append([]string{"group", "read", "1/2/7", "--routing", "--source", "1.1.252", "--dpt", "1.001"}, link...)...)
	want(t, r, 0, "1.1.251", "(on)")
	want(t, b.wait(t), 0, "answered 1 reads")
	want(t, run(t, append([]string{"group", "respond", "1/2/7", "off", "--routing"}, link...)...), 0, "response 1/2/7 00 sent via routing")
}
