// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/otfabric/knxctl/internal/errs"
)

func TestDPTList(t *testing.T) {
	r := run(t, "dpt", "list")
	want(t, r, 0, "1.001", "DPT_Switch", "9.001", "DPT_Value_Temp", "[°C]")
	r = run(t, "dpt", "list", "--main", "9", "--format", "table")
	want(t, r, 0, "ID", "NAME", "UNIT", "BITS", "9.001")
	if strings.Contains(r.out, "1.001") {
		t.Fatalf("--main 9 lists 1.001:\n%s", r.out)
	}
	res := resultOf(t, run(t, "dpt", "list", "--main", "20", "--format", "json"), "dpt list")
	types := res["types"].([]any)
	if len(types) == 0 {
		t.Fatal("no types")
	}
	first := types[0].(map[string]any)
	if !strings.HasPrefix(first["id"].(string), "20.") || first["kind"] == nil || first["bits"] == nil {
		t.Fatalf("type %v", first)
	}
	// An unknown main number lists nothing, as an empty array.
	res = resultOf(t, run(t, "dpt", "list", "--main", "250", "--format", "json"), "dpt list")
	if l := res["types"].([]any); len(l) != 0 {
		t.Fatalf("types %v", l)
	}
}

func TestDPTDescribe(t *testing.T) {
	want(t, run(t, "dpt", "describe", "1.001"), 0, "DPT_Switch", "1 bits", "0 = off, 1 = on")
	want(t, run(t, "dpt", "describe", "9.001"), 0, "temperature", "unit °C", "range -273 .. 670433.28")
	want(t, run(t, "dpt", "describe", "20.102"), 0, "values: 0=auto, 1=comfort")
	res := resultOf(t, run(t, "dpt", "describe", "9.001", "--format", "json"), "dpt describe")
	if res["id"] != "9.001" || res["unit"] != "°C" || res["min"] == nil || res["max"] == nil {
		t.Fatalf("describe %v", res)
	}
	res = resultOf(t, run(t, "dpt", "describe", "20.102", "--format", "json"), "dpt describe")
	if v := res["values"].([]any); len(v) < 5 || v[1] != "1=comfort" {
		t.Fatalf("values %v", v)
	}
	// A table falls back to text: describe has no columns.
	want(t, run(t, "dpt", "describe", "1.001", "--format", "table"), 0, "DPT_Switch")
	r := run(t, "dpt", "describe", "999.999")
	want(t, r, errs.ExitInvalidInput, "datapoint type \"999.999\"", "knxctl dpt list")
	e := errorOf(t, run(t, "dpt", "describe", "nope", "--format", "json"))
	if e["kind"] != "invalid_input" || e["code"] != "invalid_input" {
		t.Fatalf("error %v", e)
	}
}

func TestDPTEncodeDecode(t *testing.T) {
	cases := []struct {
		dpt, value, hex, text string
		small                 bool
	}{
		{"1.001", "on", "01", "on", true},
		{"1.001", "off", "00", "off", true},
		{"3.007", "+5", "0D", "+5", true},
		{"3.007", "-3", "03", "-3", true},
		{"5.001", "100", "FF", "100.0 %", false},
		{"9.001", "21.5", "0C33", "21.5 °C", false},
		{"9.001", "-30", "8A24", "-30 °C", false},
		{"10.001", "Mon 12:30:00", "2C1E00", "Mon 12:30:00", false},
		{"14.056", "1234.5", "449A5000", "1234.5 W", false},
		{"16.000", "hello", "68656C6C6F000000000000000000", `"hello"`, false},
		{"20.102", "comfort", "01", "comfort", false},
		{"232.600", "#FF8000", "FF8000", "#FF8000", false},
	}
	for _, c := range cases {
		args := []string{"dpt", "encode", c.dpt, "--", c.value}
		r := run(t, args...)
		want(t, r, 0, c.hex, "(DPT "+c.dpt+")")
		if c.small != strings.Contains(r.out, "sent in the APCI octet") {
			t.Fatalf("%s %s: small %t\n%s", c.dpt, c.value, c.small, r.out)
		}
		if c.text != "" && !strings.Contains(r.out, c.text) {
			t.Fatalf("%s %s: lacks %q\n%s", c.dpt, c.value, c.text, r.out)
		}
		r = run(t, "dpt", "decode", c.dpt, c.hex, "--format", "json")
		res := resultOf(t, r, "dpt decode")
		if res["dpt"] != c.dpt || res["data"] != c.hex || res["small"] != c.small || res["value"] == nil {
			t.Fatalf("decode %s %s: %v", c.dpt, c.hex, res)
		}
		if c.text != "" && res["text"] != c.text {
			t.Fatalf("decode %s %s: text %v, want %q", c.dpt, c.hex, res["text"], c.text)
		}
	}
	// Structured values keep their fields in JSON.
	res := resultOf(t, run(t, "dpt", "encode", "20.102", "comfort", "--format", "json"), "dpt encode")
	if v := res["value"].(map[string]any); v["name"] != "comfort" || v["value"] != 1.0 {
		t.Fatalf("enum %v", v)
	}
	res = resultOf(t, run(t, "dpt", "encode", "9.001", "21.5", "--format", "json"), "dpt encode")
	if res["value"] != 21.5 || res["data"] != "0C33" {
		t.Fatalf("encode %v", res)
	}
	// Tables fall back to text for codec results.
	want(t, run(t, "dpt", "decode", "9.001", "0x0C 33", "--format", "table"), 0, "0C33  21.5 °C")
	for _, d := range []string{"11.001", "19.001", "251.600", "13.001"} {
		want(t, run(t, "dpt", "describe", d), 0, d)
	}
}

func TestDPTInvalid(t *testing.T) {
	cases := []struct {
		args []string
		code string
	}{
		{[]string{"dpt", "encode", "9.001", "warm"}, "invalid_value"},
		{[]string{"dpt", "encode", "9.001", "1e9"}, "invalid_value"},
		{[]string{"dpt", "encode", "5.001", "101"}, "invalid_value"},
		{[]string{"dpt", "encode", "0.000", "1"}, "invalid_input"},
		{[]string{"dpt", "decode", "9.001", "zz"}, "invalid_value"},
		{[]string{"dpt", "decode", "9.001", "0C"}, "invalid_value"},
		{[]string{"dpt", "decode", "nope", "0C"}, "invalid_input"},
	}
	for _, c := range cases {
		want(t, run(t, c.args...), errs.ExitInvalidInput, "error:")
		r := run(t, append(c.args, "--format", "json")...)
		want(t, r, errs.ExitInvalidInput)
		if e := errorOf(t, r); e["kind"] != "invalid_input" || e["code"] != c.code {
			t.Fatalf("%v: error %v", c.args, e)
		}
	}
	want(t, run(t, "dpt", "encode", "9.001"), errs.ExitUsage, "accepts 2 arg(s)")
}
