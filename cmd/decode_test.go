// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"net/netip"
	"strings"
	"testing"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/go-knx/apdu"
	"github.com/otfabric/go-knx/cemi"
	"github.com/otfabric/go-knx/knxnetip"
	"github.com/otfabric/knxctl/internal/errs"
)

func frameHex(t *testing.T, m knxnetip.Message) string {
	t.Helper()
	b, err := knxnetip.Marshal(m)
	if err != nil {
		t.Fatalf("marshal %T: %v", m, err)
	}
	return hex.EncodeToString(b)
}

func groupCEMI(t *testing.T, code cemi.MessageCode, tpdu []byte) []byte {
	t.Helper()
	f := cemi.NewGroupFrame(code, knx.MustParseIndividualAddress("1.1.10"), knx.MustParseGroupAddress("1/2/3"), knx.PriorityLow, tpdu)
	b, err := cemi.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeKNXnetIP(t *testing.T) {
	r := run(t, "decode", "061004200015040100001100bce0110a0a03010081")
	want(t, r, 0, "KNXnet/IP TUNNELLING_REQUEST (21 octets)", "channel: 1", "L_Data.req 1.1.10 -> 1/2/3", "A_GroupValue_Write 01")

	res := resultOf(t, run(t, "decode", "0x0610 0420 0015 04010000 1100bce0110a0a03010081", "--format", "json"), "decode")
	if res["layer"] != "KNXnet/IP" || res["service"] != "TUNNELLING_REQUEST" || res["length"] != 21.0 {
		t.Fatalf("decode %v", res)
	}
	c := res["cemi"].(map[string]any)
	if c["messageCode"] != "L_Data.req" || c["source"] != "1.1.10" || c["destination"] != "1/2/3" || c["apci"] != "A_GroupValue_Write" || c["data"] != "01" {
		t.Fatalf("cemi %v", c)
	}

	write, _ := apdu.GroupValueWrite(apdu.GroupValue{Data: []byte{0x0C, 0x33}})
	ind := groupCEMI(t, cemi.LDataInd, write)
	h := knxnetip.NewHPAI(netip.MustParseAddrPort("192.168.1.10:3671"))
	ia := knx.MustParseIndividualAddress("1.1.10")
	dibs := knxnetip.DIBs{
		knxnetip.DeviceInfo{Medium: knxnetip.MediumTP1, Status: 1, Address: knx.MustParseIndividualAddress("15.15.0"), Name: "gw", Multicast: netip.MustParseAddr("224.0.23.12")},
		knxnetip.ServiceFamilies{Families: []knxnetip.ServiceFamily{{ID: knxnetip.FamilyCore, Version: 1}, {ID: knxnetip.FamilyTunnelling, Version: 1}}},
		knxnetip.ServiceFamilies{Secured: true, Families: []knxnetip.ServiceFamily{{ID: knxnetip.FamilyTunnelling, Version: 1}}},
		knxnetip.TunnellingInfo{MaxAPDU: 254, Slots: []knxnetip.TunnelSlot{{Address: ia, Status: 7}, {Address: knx.MustParseIndividualAddress("1.1.11"), Status: 6}}},
		knxnetip.KNXAddresses{Addresses: []knx.IndividualAddress{ia}},
		knxnetip.IPConfig{IP: netip.MustParseAddr("192.168.1.10"), Mask: netip.MustParseAddr("255.255.255.0"), Gateway: netip.MustParseAddr("192.168.1.1")},
		knxnetip.IPCurrentConfig{IP: netip.MustParseAddr("192.168.1.10"), Mask: netip.MustParseAddr("255.255.255.0"), Gateway: netip.MustParseAddr("192.168.1.1"), DHCPServer: netip.MustParseAddr("192.168.1.1")},
	}
	cases := []struct {
		m    knxnetip.Message
		want []string
	}{
		{&knxnetip.TunnellingAck{Channel: 2, Seq: 7}, []string{"TUNNELLING_ACK", "sequence: 7"}},
		{&knxnetip.RoutingIndication{CEMI: ind}, []string{"ROUTING_INDICATION", "L_Data.ind", "0C33"}},
		{&knxnetip.DeviceConfigurationRequest{Channel: 3, Seq: 1, CEMI: ind}, []string{"DEVICE_CONFIGURATION_REQUEST", "channel: 3"}},
		{&knxnetip.ConnectRequest{Control: h, Data: h, CRI: knxnetip.CRI{Type: knxnetip.TunnelConnection, Layer: knxnetip.TunnelLinkLayer}}, []string{"CONNECT_REQUEST", "connectionType", "192.168.1.10:3671"}},
		{&knxnetip.ConnectResponse{Channel: 4, Data: h, CRD: knxnetip.CRD{Type: knxnetip.TunnelConnection, Address: ia}}, []string{"CONNECT_RESPONSE", "address: 1.1.10"}},
		{&knxnetip.ConnectResponse{Status: knxnetip.StatusNoMoreConnections}, []string{"CONNECT_RESPONSE", "status:"}},
		{&knxnetip.ConnectionStateRequest{Channel: 5, Control: h}, []string{"CONNECTIONSTATE_REQUEST", "channel: 5"}},
		{&knxnetip.ConnectionStateResponse{Channel: 5}, []string{"CONNECTIONSTATE_RESPONSE"}},
		{&knxnetip.DisconnectRequest{Channel: 6, Control: h}, []string{"DISCONNECT_REQUEST", "channel: 6"}},
		{&knxnetip.DisconnectResponse{Channel: 6}, []string{"DISCONNECT_RESPONSE"}},
		{&knxnetip.SearchRequest{Discovery: h}, []string{"SEARCH_REQUEST", "discovery:"}},
		{&knxnetip.DescriptionRequest{Control: h}, []string{"DESCRIPTION_REQUEST", "control:"}},
		{&knxnetip.SearchResponse{Control: h, DIBs: dibs}, []string{"SEARCH_RESPONSE", "device:"}},
		{&knxnetip.DescriptionResponse{DIBs: dibs}, []string{"DESCRIPTION_RESPONSE", "device:"}},
	}
	// The device blocks, as describe and discover print them.
	res0 := resultOf(t, run(t, "decode", frameHex(t, &knxnetip.DescriptionResponse{DIBs: dibs}), "--format", "json"), "decode")
	dev := res0["fields"].(map[string]any)["device"].(map[string]any)
	if dev["programmingMode"] != true || dev["multicast"] != "224.0.23.12" || dev["securedServices"] == nil ||
		len(dev["tunnels"].([]any)) != 2 || len(dev["knxAddresses"].([]any)) != 1 || dev["ip"].(map[string]any)["currentIP"] != "192.168.1.10" {
		t.Fatalf("device %v", dev)
	}
	text := newDeviceRecord("192.168.1.10:3671", dibs).text()
	for _, w := range []string{"routing multicast 224.0.23.12", "programming mode on", "secured services: Tunnelling v1", "tunnel 1.1.10 free", "tunnel 1.1.11 in use", "KNX addresses: 1.1.10", "configuredIP 192.168.1.10"} {
		if !strings.Contains(text, w) {
			t.Fatalf("text lacks %q:\n%s", w, text)
		}
	}
	for _, c := range cases {
		x := frameHex(t, c.m)
		want(t, run(t, "decode", x), 0, c.want...)
		res := resultOf(t, run(t, "decode", x, "--format", "json"), "decode")
		if res["layer"] != "KNXnet/IP" || res["service"] == "" {
			t.Fatalf("%T: %v", c.m, res)
		}
	}
	// A frame without fields has none in JSON.
	res = resultOf(t, run(t, "decode", frameHex(t, &knxnetip.RoutingIndication{CEMI: ind}), "--format", "json"), "decode")
	if _, ok := res["fields"]; ok {
		t.Fatalf("fields %v", res["fields"])
	}
	// An unknown service is shown with its body.
	want(t, run(t, "decode", "06100999000a01020304"), 0, "body: 01020304")
	// A tunnelling request whose cEMI does not parse reports it.
	bad := frameHex(t, &knxnetip.TunnellingRequest{Channel: 1, CEMI: []byte{0x11}})
	res = resultOf(t, run(t, "decode", bad, "--format", "json"), "decode")
	if f := res["fields"].(map[string]any); f["cemiError"] == nil {
		t.Fatalf("fields %v", f)
	}
}

func TestDecodeCEMI(t *testing.T) {
	want(t, run(t, "decode", "--cemi", "2900bce0110a0a03010081"), 0, "cEMI L_Data.ind (11 octets)", "1.1.10 -> 1/2/3", "A_GroupValue_Write 01")
	read := groupCEMI(t, cemi.LDataReq, apdu.GroupValueRead())
	res := resultOf(t, run(t, "decode", "--cemi", hex.EncodeToString(read), "--format", "json"), "decode")
	c := res["cemi"].(map[string]any)
	if res["layer"] != "cEMI" || c["apci"] != "A_GroupValue_Read" || c["messageCode"] != "L_Data.req" || c["standardFrame"] != true {
		t.Fatalf("decode %v", res)
	}
	// A control TPDU (T_Connect) has no APCI.
	f := cemi.NewGroupFrame(cemi.LDataInd, knx.MustParseIndividualAddress("1.1.10"), knx.MustParseGroupAddress("1/2/3"), knx.PriorityLow, []byte{0x80})
	f.Ctrl2 &^= 0x80 // individual destination
	b, _ := cemi.Marshal(f)
	r := run(t, "decode", "--cemi", hex.EncodeToString(b), "--format", "json")
	want(t, r, 0)
	if c := resultOf(t, r, "decode")["cemi"].(map[string]any); c["apci"] != nil || c["tpci"] == nil {
		t.Fatalf("control frame %v", c)
	}
	// A non-L_Data frame has only its message code.
	want(t, run(t, "decode", "--cemi", "fc000001000101"), 0, "cEMI")
}

func TestDecodeInvalid(t *testing.T) {
	want(t, run(t, "decode", "zz"), errs.ExitInvalidInput, "want hexadecimal octets")
	r := run(t, "decode", "0610", "--format", "json")
	want(t, r, errs.ExitProtocol)
	if e := errorOf(t, r); e["kind"] != "protocol" || e["code"] != "malformed_frame" {
		t.Fatalf("error %v", e)
	}
	r = run(t, "decode", "--cemi", "29")
	want(t, r, errs.ExitProtocol, "error:")
	if !strings.Contains(r.errOut, "error:") {
		t.Fatal(r.errOut)
	}
}
