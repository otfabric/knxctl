// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/cemi"
	"github.com/otfabric/go-knx/client"
	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/go-knx/tunnel"
)

// Chaos tests: knxctl against hostile KNXnet/IP peers. A peer that
// answers with garbage, truncated and oversized datagrams and floods, or
// that accepts a connection and then never answers, must not hang a
// command past its --timeout, crash it or corrupt its output: the command
// ends with its documented exit code.

// hostilePeer is a UDP KNXnet/IP peer on 127.0.0.1 whose handler decides
// what to answer.
type hostilePeer struct {
	t      *testing.T
	conn   *net.UDPConn
	addr   string
	handle func(p *hostilePeer, from netip.AddrPort, m knxnetip.Message)
	wg     sync.WaitGroup
	got    atomic.Int64 // datagrams received
}

func startHostile(t *testing.T, handle func(p *hostilePeer, from netip.AddrPort, m knxnetip.Message)) *hostilePeer {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	// Big buffers: a flood from the command (acknowledgements) must not
	// overrun the peer on a loaded CI runner.
	_ = c.SetReadBuffer(4 << 20)
	_ = c.SetWriteBuffer(4 << 20)
	p := &hostilePeer{t: t, conn: c, addr: c.LocalAddr().String(), handle: handle}
	p.wg.Add(1)
	go p.loop()
	t.Cleanup(func() {
		_ = c.Close()
		p.wg.Wait()
	})
	return p
}

func (p *hostilePeer) loop() {
	defer p.wg.Done()
	buf := make([]byte, 64<<10)
	for {
		n, from, err := p.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// Windows reports an ICMP port unreachable of an earlier
			// datagram as a read error: read again.
			continue
		}
		p.got.Add(1)
		m, err := knxnetip.Parse(buf[:n])
		if err != nil {
			m = nil
		}
		p.handle(p, from, m)
	}
}

func (p *hostilePeer) raw(to netip.AddrPort, b []byte) {
	_, _ = p.conn.WriteToUDPAddrPort(b, to)
}

func (p *hostilePeer) send(to netip.AddrPort, m knxnetip.Message) {
	b, err := knxnetip.Marshal(m)
	if err != nil {
		p.t.Errorf("marshal %T: %v", m, err)
		return
	}
	p.raw(to, b)
}

// noise answers every datagram with garbage: random octets, a frame
// whose header claims more than it holds, a header of a wrong version,
// an oversized datagram, a response of the wrong service and a burst.
func noise(p *hostilePeer, from netip.AddrPort, _ knxnetip.Message) {
	r := rand.New(rand.NewPCG(uint64(from.Port()), 7))
	junk := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(r.Uint32())
		}
		return b
	}
	p.raw(from, junk(64))
	// SEARCH_RESPONSE claiming 0x0040 octets, holding 10.
	p.raw(from, []byte{0x06, 0x10, 0x02, 0x02, 0x00, 0x40, 0x08, 0x01, 0x7f, 0x00})
	// DESCRIPTION_RESPONSE truncated in the middle of its DEVICE_INFO.
	p.raw(from, []byte{0x06, 0x10, 0x02, 0x04, 0x00, 0x0c, 0x36, 0x01, 0x02, 0x00, 0x11, 0x01})
	// CONNECT_RESPONSE of a wrong protocol version.
	p.raw(from, []byte{0x06, 0x20, 0x02, 0x06, 0x00, 0x08, 0x01, 0x00})
	// An oversized datagram: a header claiming 0xFFFF octets, 60000 sent.
	big := junk(60000)
	copy(big, []byte{0x06, 0x10, 0x02, 0x02, 0xff, 0xff})
	p.raw(from, big)
	// A well-formed answer nobody asked for.
	p.send(from, &knxnetip.ConnectionStateResponse{Channel: 0xEE, Status: knxnetip.StatusNoError})
	for range 500 {
		p.raw(from, junk(1+r.IntN(80)))
	}
}

// within runs knxctl and fails the test when it does not end within max.
func within(t *testing.T, max time.Duration, args ...string) result {
	t.Helper()
	t0 := time.Now()
	b := start(t, args...)
	select {
	case code := <-b.done:
		b.done <- code
	case <-time.After(max):
		t.Fatalf("%v still running after %v (hang)\nstdout: %s\nstderr: %s", args, max, b.out.String(), b.errOut.String())
	}
	r := b.wait(t)
	t.Logf("%v: exit %d in %v", args[:2], r.code, time.Since(t0).Round(time.Millisecond))
	return r
}

// validJSON fails the test unless out is JSON documents only (one
// indented document, or JSON Lines).
func validJSON(t *testing.T, out string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err.Error() == "EOF" {
				return
			}
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
	}
}

func TestChaosNoisePeer(t *testing.T) {
	t.Parallel()
	p := startHostile(t, noise)
	const timeout = "800ms"
	cases := []struct {
		code int
		args []string
	}{
		{0, []string{"discover", "--unicast", p.addr}},
		{5, []string{"describe", p.addr}},
		{5, []string{"group", "read", "1/2/3", "-g", p.addr}},
		{5, []string{"group", "write", "1/2/3", "on", "-g", p.addr}},
		{5, []string{"property", "read", p.addr, "device", "11"}},
		{5, []string{"property", "read", p.addr, "device", "11", "--device", "1.1.5"}},
		{5, []string{"tunnel", "connect", p.addr}},
		{5, []string{"tunnel", "feature", "get", p.addr, "max-apdu", "--udp"}},
		{5, []string{"busmonitor", p.addr}},
		{5, []string{"monitor", "-g", p.addr}},
		{1, []string{"diagnose", p.addr}},
	}
	for _, c := range cases {
		args := append(append([]string{}, c.args...), "--timeout", timeout, "--format", "json")
		r := within(t, 10*time.Second, args...)
		want(t, r, c.code)
		validJSON(t, r.out)
		if c.args[0] == "discover" && !strings.Contains(r.out, `"devices": []`) {
			t.Fatalf("discover found a device in noise:\n%s", r.out)
		}
	}
	if p.got.Load() == 0 {
		t.Fatal("the noise peer was never contacted")
	}
}

// acceptor is a peer that accepts tunnelling and device management
// connections, acknowledges every request and answers nothing else; with
// flood, it then sends n group responses (and garbage cEMI) to the client.
type acceptor struct {
	mu        sync.Mutex
	flood     int
	busmon    bool
	connected atomic.Int64
}

func (a *acceptor) handle(p *hostilePeer, from netip.AddrPort, m knxnetip.Message) {
	switch m := m.(type) {
	case *knxnetip.ConnectRequest:
		a.connected.Add(1)
		crd := knxnetip.CRD{Type: m.CRI.Type}
		if m.CRI.Type == knxnetip.TunnelConnection {
			crd.Address = knx.MustParseIndividualAddress("1.1.200")
		}
		data := m.Data.Addr
		if !data.IsValid() || data.Addr().IsUnspecified() {
			data = from
		}
		p.send(from, &knxnetip.ConnectResponse{Channel: 7, Status: knxnetip.StatusNoError,
			Data: knxnetip.NewHPAI(netip.MustParseAddrPort(p.addr)), CRD: crd})
		a.mu.Lock()
		n, busmon := a.flood, a.busmon
		a.mu.Unlock()
		if n > 0 {
			go floodClient(p, data, n, busmon)
		}
	case *knxnetip.ConnectionStateRequest:
		p.send(from, &knxnetip.ConnectionStateResponse{Channel: m.Channel, Status: knxnetip.StatusNoError})
	case *knxnetip.TunnellingRequest:
		p.send(from, &knxnetip.TunnellingAck{Channel: m.Channel, Seq: m.Seq, Status: knxnetip.StatusNoError})
	case *knxnetip.DeviceConfigurationRequest:
		p.send(from, &knxnetip.DeviceConfigurationAck{Channel: m.Channel, Seq: m.Seq, Status: knxnetip.StatusNoError})
	case *knxnetip.TunnellingFeature:
		p.send(from, &knxnetip.TunnellingAck{Channel: m.Channel, Seq: m.Seq, Status: knxnetip.StatusNoError})
	case *knxnetip.DisconnectRequest:
		p.send(from, &knxnetip.DisconnectResponse{Channel: m.Channel, Status: knxnetip.StatusNoError})
	}
}

// floodClient sends n telegrams to the client: group responses to 1/2/4
// from changing sources (L_Data.ind, or L_Busmon.ind), with garbage cEMI
// frames between them.
func floodClient(p *hostilePeer, to netip.AddrPort, n int, busmon bool) {
	time.Sleep(50 * time.Millisecond) // let the client finish connecting
	g := knx.MustParseGroupAddress("1/2/4")
	tpdu, _ := apdu.GroupValueResponse(apdu.GroupValue{Data: []byte{0x0C, 0x33}})
	for i := range n {
		var b []byte
		f := cemi.NewGroupFrame(cemi.LDataInd, knx.IndividualAddress(0x1100+uint16(i%200)), g, knx.PriorityLow, tpdu)
		if busmon {
			tp1, err := cemi.TP1Frame(f)
			if err != nil {
				p.t.Error(err)
				return
			}
			b, _ = cemi.Marshal(&cemi.Raw{MsgCode: cemi.LBusmonInd, Data: tp1})
		} else {
			b, _ = cemi.Marshal(f)
		}
		if i%10 == 9 {
			b = []byte{0x29, 0x00, 0xbc} // truncated L_Data.ind
		}
		p.send(to, &knxnetip.TunnellingRequest{Channel: 7, Seq: uint8(i), CEMI: b})
	}
}

// A server that accepts the connection and then answers nothing: every
// command ends within its --timeout with its documented exit code.
func TestChaosSilentAfterConnect(t *testing.T) {
	t.Parallel()
	a := &acceptor{}
	p := startHostile(t, a.handle)
	cases := []struct {
		codes []int
		max   time.Duration
		args  []string
	}{
		{[]int{5}, 4 * time.Second, []string{"group", "read", "1/2/4", "-g", p.addr}},
		{[]int{5}, 4 * time.Second, []string{"group", "read", "1/2/4", "--all", "300ms", "-g", p.addr}},
		// L_Data.con never comes: not confirmed (6) or the deadline (5).
		{[]int{5, 6}, 5 * time.Second, []string{"group", "write", "1/2/3", "on", "-g", p.addr}},
		// Local device management: the M_PropRead.con never comes. go-knx
		// waits as long as the context lives; this hung for ever.
		{[]int{5}, 4 * time.Second, []string{"property", "read", p.addr, "device", "11"}},
		{[]int{5}, 4 * time.Second, []string{"property", "write", p.addr, "device", "54", "01"}},
		// Remote device management is bounded by --timeout plus 5 s.
		{[]int{5, 6}, 10 * time.Second, []string{"property", "read", p.addr, "device", "11", "--device", "1.1.5"}},
		// The feature services waited a fixed 3 s.
		{[]int{5}, 2500 * time.Millisecond, []string{"tunnel", "feature", "get", p.addr, "max-apdu", "--udp"}},
		{[]int{0}, 4 * time.Second, []string{"monitor", "-g", p.addr, "--duration", "300ms"}},
		{[]int{0}, 4 * time.Second, []string{"busmonitor", p.addr, "--duration", "300ms"}},
	}
	for _, c := range cases {
		args := append(append([]string{}, c.args...), "--timeout", "800ms", "--format", "json")
		r := within(t, c.max, args...)
		ok := false
		for _, code := range c.codes {
			ok = ok || r.code == code
		}
		if !ok {
			t.Fatalf("%v: exit %d, want one of %v\nstdout: %s\nstderr: %s", c.args, r.code, c.codes, r.out, r.errOut)
		}
		validJSON(t, r.out)
	}
	if a.connected.Load() == 0 {
		t.Fatal("no command connected")
	}
}

// A server that floods the client with telegrams after connecting.
func TestChaosFloodAfterConnect(t *testing.T) {
	t.Parallel()
	a := &acceptor{flood: 2000}
	p := startHostile(t, a.handle)

	// group read --all keeps at most maxReadResponses; whatever the
	// socket buffers let through, the result is one JSON document.
	r := within(t, 10*time.Second, "group", "read", "1/2/4", "--all", "1s", "-g", p.addr, "--timeout", "2s", "--format", "json")
	switch r.code {
	case 0:
		res := resultOf(t, r, "group read")
		if n := len(res["responses"].([]any)); n < 1 || n > maxReadResponses {
			t.Fatalf("%d responses", n)
		}
	case 5: // every datagram of the flood lost (an overloaded runner)
	default:
		t.Fatalf("exit %d\n%s%s", r.code, r.out, r.errOut)
	}

	// Streams stop at --count, garbage cEMI among the telegrams.
	r = within(t, 10*time.Second, "monitor", "-g", p.addr, "--count", "25", "--duration", "5s", "--timeout", "2s", "--format", "json")
	want(t, r, 0)
	if lines := jsonLines(t, r.out); len(lines) > 25 {
		t.Fatalf("%d records for --count 25", len(lines))
	}
	a.mu.Lock()
	a.busmon = true
	a.mu.Unlock()
	r = within(t, 10*time.Second, "busmonitor", p.addr, "--count", "25", "--duration", "5s", "--timeout", "2s", "--format", "json")
	want(t, r, 0)
	if lines := jsonLines(t, r.out); len(lines) > 25 {
		t.Fatalf("%d records for --count 25", len(lines))
	}
}

// readAll keeps at most limit responses of a flood and counts the rest.
func TestReadAllBounded(t *testing.T) {
	t.Parallel()
	a := &acceptor{flood: 400}
	p := startHostile(t, a.handle)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Read retries: a burst can overrun the socket buffers under -race
	// on a loaded runner, so a few attempts may see nothing.
	for attempt := range 5 {
		c, err := client.DialTunnel(ctx, p.addr, []tunnel.Option{tunnel.WithConnectTimeout(2 * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		rs, dropped, err := readAll(ctx, c, knx.MustParseGroupAddress("1/2/4"), 700*time.Millisecond, 3)
		_ = c.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(rs) > 3 {
			t.Fatalf("%d responses kept, limit 3", len(rs))
		}
		if len(rs) == 3 && dropped > 0 {
			return
		}
		t.Logf("attempt %d: %d kept, %d dropped", attempt, len(rs), dropped)
	}
	t.Fatal("the flood never exceeded the limit")
}

// The SEARCH_RESPONSE of a directed search, followed by noise: discover
// takes the answer and ignores the rest.
func TestChaosDiscoverAnswerAmidNoise(t *testing.T) {
	t.Parallel()
	p := startHostile(t, func(p *hostilePeer, from netip.AddrPort, m knxnetip.Message) {
		if _, ok := m.(*knxnetip.SearchRequest); !ok {
			return
		}
		noise(p, from, m)
		dev := knxnetip.DeviceInfo{Medium: knxnetip.MediumIP, Address: knx.MustParseIndividualAddress("1.1.0"), Name: "hostile"}
		binary.BigEndian.PutUint16(dev.Serial[:], 0x00FA)
		p.send(from, &knxnetip.SearchResponse{Control: knxnetip.NewHPAI(netip.MustParseAddrPort(p.addr)),
			DIBs: knxnetip.DIBs{dev, knxnetip.ServiceFamilies{Families: []knxnetip.ServiceFamily{{ID: knxnetip.FamilyCore, Version: 1}}}}})
	})
	// Read retries: the noise burst before the answer can overrun the
	// socket buffer.
	for range 5 {
		r := within(t, 10*time.Second, "discover", "--unicast", p.addr, "--timeout", "1s", "--format", "json")
		want(t, r, 0)
		if strings.Contains(r.out, `"hostile"`) {
			return
		}
	}
	t.Fatal("the answer was never found among the noise")
}
