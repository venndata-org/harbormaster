package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sample() *State {
	created := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	seen := time.Date(2026, 6, 18, 11, 30, 0, 0, time.UTC)
	s := New()
	s.Instances["/Users/av/dev-vd/groundtruth/feat-x"] = &Instance{
		Project:    "groundtruth",
		Label:      "feat-x",
		Block:      [2]int{20020, 20039},
		Berths:     map[string]int{"tilt": 20020, "web": 20021, "api": 20022},
		CreatedAt:  created,
		LastSeenAt: seen,
	}
	return s
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := sample()
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got %+v\n want %+v", got.Instances, want.Instances)
	}
}

func TestSave_ShapeMatchesSpec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Save(path, sample()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{
		`"version": 1`,
		`"instances"`,
		`"block": [`,
		`"berths"`,
		`"tilt": 20020`,
		`"createdAt": "2026-06-18T10:00:00Z"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("state.json missing %q\n---\n%s", want, out)
		}
	}
}

func TestLoad_Missing(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if got.Version != Version || len(got.Instances) != 0 {
		t.Fatalf("missing file should yield empty current-version state, got %+v", got)
	}
}

func TestLoad_Corrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error for corrupt json")
	}
}

func TestSave_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "state.json")
	if err := Save(path, New()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state.json not created: %v", err)
	}
}

func TestSave_NoTempLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	for i := 0; i < 3; i++ {
		if err := Save(path, sample()); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "state.json" {
		t.Fatalf("expected only state.json, found %v", names)
	}
}
