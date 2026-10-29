// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type res struct {
	A string `json:"a"`
}

func (r res) Text() string      { return "a=" + r.A + "\n" }
func (r res) Headers() []string { return []string{"A", "B"} }
func (r res) Rows() [][]string  { return [][]string{{r.A, "x"}} }

type plain struct{}

func (plain) Text() string { return "plain" }

func TestParse(t *testing.T) {
	for in, want := range map[string]Format{"": Text, "text": Text, " JSON ": JSON, "table": Table} {
		if f, err := Parse(in); err != nil || f != want {
			t.Errorf("Parse(%q) = %v, %v", in, f, err)
		}
	}
	if _, err := Parse("xml"); err == nil || !strings.Contains(err.Error(), "text, json or table") {
		t.Fatalf("Parse(xml): %v", err)
	}
}

func TestWrite(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, Text, "c", res{"1"}); err != nil || b.String() != "a=1\n" {
		t.Fatalf("text %q %v", b.String(), err)
	}
	b.Reset()
	if err := Write(&b, Table, "c", res{"1"}); err != nil || b.String() != "A  B\n1  x\n" {
		t.Fatalf("table %q %v", b.String(), err)
	}
	b.Reset()
	if err := Write(&b, Table, "c", plain{}); err != nil || b.String() != "plain\n" {
		t.Fatalf("table fallback %q %v", b.String(), err)
	}
	b.Reset()
	if err := Write(&b, JSON, "c", res{"1"}); err != nil {
		t.Fatal(err)
	}
	var env struct {
		SchemaVersion string `json:"schemaVersion"`
		Command       string `json:"command"`
		Result        res    `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &env); err != nil || env.SchemaVersion != SchemaVersion || env.Command != "c" || env.Result.A != "1" {
		t.Fatalf("json %s %v", b.String(), err)
	}
	b.Reset()
	if err := WriteError(&b, map[string]string{"kind": "usage"}); err != nil || !strings.Contains(b.String(), `"error": {`) {
		t.Fatalf("error %s %v", b.String(), err)
	}
	// Values JSON cannot encode.
	if err := Write(&b, JSON, "c", badJSON{}); err == nil {
		t.Fatal("no error for an unencodable result")
	}
}

type badJSON struct{ C chan int }

func (badJSON) Text() string { return "" }

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestStream(t *testing.T) {
	var b bytes.Buffer
	s := NewStream(&b, JSON)
	if err := s.Record("telegram", "ignored", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Record("empty", "ignored", struct{}{}); err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":"1.0","type":"telegram","n":1}` + "\n" + `{"schemaVersion":"1.0","type":"empty"}` + "\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
	if err := s.Record("bad", "", badJSON{}); err == nil {
		t.Fatal("no error for an unencodable record")
	}
	b.Reset()
	if err := NewStream(&b, Table).Record("t", "line", nil); err != nil || b.String() != "line\n" {
		t.Fatalf("text %q %v", b.String(), err)
	}
	if err := NewStream(failWriter{}, Text).Record("t", "line", nil); err == nil {
		t.Fatal("no write error")
	}
	if err := Write(failWriter{}, JSON, "c", res{}); err == nil {
		t.Fatal("no write error")
	}
}
