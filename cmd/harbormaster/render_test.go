package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/venndata-org/harbormaster/internal/config"
	"github.com/venndata-org/harbormaster/internal/gitident"
	"github.com/venndata-org/harbormaster/internal/ipc"
)

func sampleResp() ipc.Response {
	blk := [2]int{20000, 20019}
	return ipc.Response{OK: true, Tilt: 20000, Block: &blk, Ports: map[string]int{"web": 20001, "api": 20002, "postgres": 20003}}
}

func TestHmEnvLines_OffsetOrder(t *testing.T) {
	got := hmEnvLines(sampleResp())
	want := []string{
		"HM_TILT_PORT=20000",
		"HM_PORT_WEB=20001",
		"HM_PORT_API=20002",
		"HM_PORT_POSTGRES=20003",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hmEnvLines =\n %v\nwant\n %v", got, want)
	}
}

func TestSortedServices(t *testing.T) {
	got := sortedServices(map[string]int{"api": 20002, "web": 20001, "db": 20003})
	want := []string{"web", "api", "db"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedServices = %v, want %v", got, want)
	}
}

func TestTiltPassthrough(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{[]string{"--", "--stream"}, []string{"--stream"}},
		{[]string{"--stream"}, []string{"--stream"}},
		{[]string{}, []string{}},
		{[]string{"--"}, []string{}},
	}
	for _, c := range cases {
		if got := tiltPassthrough(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("tiltPassthrough(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestServiceList_Set(t *testing.T) {
	var s serviceList
	for _, v := range []string{"web:3000", "api:4000", "bare"} {
		if err := s.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	want := serviceList{{"web", 3000}, {"api", 4000}, {"bare", 0}}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("serviceList = %v, want %v", s, want)
	}
	for _, bad := range []string{"", ":3000", "x:notaport"} {
		var z serviceList
		if err := z.Set(bad); err == nil {
			t.Errorf("Set(%q) should error", bad)
		}
	}
}

func TestRenderProjectTOML_WithServices_Parses(t *testing.T) {
	body := renderProjectTOML("groundtruth", serviceList{{"web", 3000}, {"api", 4000}})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "harbormaster.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, found, err := config.LoadProject(dir)
	if err != nil || !found {
		t.Fatalf("generated toml should parse: found=%v err=%v\n%s", found, err, body)
	}
	if p.Name != "groundtruth" {
		t.Errorf("name = %q", p.Name)
	}
	if p.Services["web"].Offset != 1 || p.Services["web"].Default != 3000 {
		t.Errorf("web = %+v", p.Services["web"])
	}
	if p.Services["api"].Offset != 2 || p.Services["api"].Default != 4000 {
		t.Errorf("api = %+v", p.Services["api"])
	}
}

func TestRenderProjectTOML_Empty_Parses(t *testing.T) {
	body := renderProjectTOML("solo", nil)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "harbormaster.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, found, err := config.LoadProject(dir)
	if err != nil || !found || p.Name != "solo" {
		t.Fatalf("empty template should parse with name: found=%v err=%v name=%q", found, err, p.Name)
	}
	if len(p.Services) != 0 {
		t.Errorf("expected no active services, got %v", p.Services)
	}
}

func TestPrintPortsJSON(t *testing.T) {
	var buf bytes.Buffer
	id := gitident.Identity{Project: "groundtruth", Instance: "/p/main", Label: "main"}
	if err := printPortsJSON(&buf, id, sampleResp()); err != nil {
		t.Fatal(err)
	}
	var doc portsDoc
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if doc.Project != "groundtruth" || doc.Tilt != 20000 || doc.Ports["web"] != 20001 {
		t.Fatalf("decoded doc wrong: %+v", doc)
	}
	if doc.Block == nil || *doc.Block != [2]int{20000, 20019} {
		t.Fatalf("block wrong: %v", doc.Block)
	}
}

func TestPrintList_Empty(t *testing.T) {
	var buf bytes.Buffer
	printList(&buf, nil)
	if got := buf.String(); got != "no active leases\n" {
		t.Fatalf("empty list = %q", got)
	}
}
