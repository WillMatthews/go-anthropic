package main

import (
	"os"
	"reflect"
	"sort"
	"testing"
)

func driftValues(d []Token) []string {
	out := make([]string, 0, len(d))
	for _, t := range d {
		out = append(out, t.Value)
	}
	sort.Strings(out)
	return out
}

func TestDiffAndAllowlist(t *testing.T) {
	ours, err := extractDir("testdata/ours", nil)
	if err != nil {
		t.Fatalf("extract ours: %v", err)
	}
	truth, err := extractDir("testdata/truth", nil)
	if err != nil {
		t.Fatalf("extract truth: %v", err)
	}

	// With no allowlist, every truth-only token is drift.
	empty := newAllowlist()
	got := driftValues(diff(ours, truth, empty))
	want := []string{
		"claude-test-beta",
		"field_two",
		"refusal",
		"type",
		"widget-feature-2025-01-01",
		"widget_block",
		"widgets/cancel",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("drift (no allowlist):\n got=%v\nwant=%v", got, want)
	}

	// The allowlist fixture suppresses field_two and refusal.
	allow, err := loadAllowlist("testdata/allowlist.txt")
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	got2 := driftValues(diff(ours, truth, allow))
	want2 := []string{
		"claude-test-beta",
		"type",
		"widget-feature-2025-01-01",
		"widget_block",
		"widgets/cancel",
	}
	if !reflect.DeepEqual(got2, want2) {
		t.Errorf("drift (with allowlist):\n got=%v\nwant=%v", got2, want2)
	}
}

func TestLoadAllowlistComments(t *testing.T) {
	allow, err := loadAllowlist("testdata/allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !allow.has("field_two") || !allow.has("refusal") {
		t.Error("expected allowlisted values present")
	}
	if allow.has("#") || allow.has("") {
		t.Error("comments/blanks must not become entries")
	}
}

func TestLoadAllowlistMissingFileIsEmpty(t *testing.T) {
	allow, err := loadAllowlist("testdata/does-not-exist.txt")
	if err != nil {
		t.Fatalf("missing allowlist should not error: %v", err)
	}
	if len(allow.values) != 0 {
		t.Error("missing allowlist should be empty")
	}
}

func TestLoadGroundTruthFromDir(t *testing.T) {
	set, source, err := loadGroundTruth(groundTruthOptions{
		dir:   "testdata/truth",
		files: []string{"truth.go"},
	})
	if err != nil {
		t.Fatalf("loadGroundTruth: %v", err)
	}
	if !set.has("claude-test-beta") {
		t.Error("expected token from ground-truth dir")
	}
	if source == "" {
		t.Error("expected a source description")
	}
}

func TestLoadGroundTruthMissingFile(t *testing.T) {
	_, _, err := loadGroundTruth(groundTruthOptions{
		dir:   "testdata/truth",
		files: []string{"nonexistent.go"},
	})
	if err == nil {
		t.Error("expected error for missing ground-truth file")
	}
}

// TestLiveFetch exercises the real git clone path. It is network-dependent and
// only runs when DRIFTCHECK_LIVE is set, so `go test ./...` stays offline-safe.
func TestLiveFetch(t *testing.T) {
	if os.Getenv("DRIFTCHECK_LIVE") == "" {
		t.Skip("set DRIFTCHECK_LIVE=1 to run the live SDK fetch test")
	}
	set, _, err := loadGroundTruth(groundTruthOptions{ref: "main"})
	if err != nil {
		t.Fatalf("live fetch: %v", err)
	}
	if !set.has("messages") {
		t.Error("expected the messages endpoint in the live SDK")
	}
}
