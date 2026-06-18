package ipc

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"
)

func TestMessage_RoundTrip(t *testing.T) {
	req := Request{Op: "lease", Instance: "/p/main", Project: "p", Label: "main", Services: []string{"web", "api"}}
	var buf bytes.Buffer
	if err := WriteMessage(&buf, req); err != nil {
		t.Fatal(err)
	}
	if buf.Bytes()[buf.Len()-1] != '\n' {
		t.Fatal("message must be newline-terminated")
	}
	var got Request
	if err := ReadMessage(bufio.NewReader(&buf), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, req) {
		t.Fatalf("round-trip mismatch:\n got %+v\n want %+v", got, req)
	}
}

func TestMessage_TwoLines(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteMessage(&buf, Response{OK: true, Version: "1"})
	_ = WriteMessage(&buf, Response{OK: false, Error: "boom"})
	r := bufio.NewReader(&buf)

	var a, b Response
	if err := ReadMessage(r, &a); err != nil {
		t.Fatal(err)
	}
	if err := ReadMessage(r, &b); err != nil {
		t.Fatal(err)
	}
	if !a.OK || a.Version != "1" {
		t.Errorf("first message wrong: %+v", a)
	}
	if b.OK || b.Error != "boom" {
		t.Errorf("second message wrong: %+v", b)
	}
}

func TestErr(t *testing.T) {
	if r := Err("nope %d", 7); r.OK || r.Error != "nope 7" {
		t.Fatalf("Err = %+v", r)
	}
	// no args: plain message passes through
	if r := Err("missing instance"); r.OK || r.Error != "missing instance" {
		t.Fatalf("Err plain = %+v", r)
	}
}

func TestIsLive_NoSocket(t *testing.T) {
	if IsLive("/tmp/harbormaster-does-not-exist.sock") {
		t.Fatal("IsLive should be false when nothing is listening")
	}
}
