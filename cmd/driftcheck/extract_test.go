package main

import (
	"sort"
	"testing"
)

func (s *TokenSet) values() []string {
	out := make([]string, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (s *TokenSet) kindOf(v string) string { return s.m[v].Kind }

func TestExtractDir(t *testing.T) {
	set, err := extractDir("testdata/truth", nil)
	if err != nil {
		t.Fatalf("extractDir: %v", err)
	}

	want := map[string]string{
		"claude-test-alpha":         KindModel,
		"claude-test-beta":          KindModel,
		"widget-feature-2025-01-01": KindBeta,
		"end_turn":                  KindConst,
		"refusal":                   KindConst,
		"field_one":                 KindJSON,
		"field_two":                 KindJSON,
		"widget_block":              KindEnum, // from `default:` tag
		"widgets":                   KindPath, // "v1/widgets" normalized
		"widgets/cancel":            KindPath, // "/widgets/cancel" normalized
	}
	for v, kind := range want {
		if !set.has(v) {
			t.Errorf("expected token %q to be extracted; have %v", v, set.values())
			continue
		}
		if got := set.kindOf(v); got != kind {
			t.Errorf("token %q: got kind %q, want %q", v, got, kind)
		}
	}

	// The "-" json tag and unexported/untagged fields must NOT produce tokens.
	if set.has("-") {
		t.Error(`json:"-" should not produce a token`)
	}
}

func TestPathToken(t *testing.T) {
	cases := []struct {
		in   string
		norm string
		ok   bool
	}{
		{"v1/messages", "messages", true},
		{"/messages", "messages", true},
		{"/messages/batches", "messages/batches", true},
		{"v1/messages/count_tokens", "messages/count_tokens", true},
		{"image/jpeg", "", false},      // media type, not a path
		{"end_turn", "", false},        // plain enum
		{"claude-opus-4-8", "", false}, // model id
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := pathToken(c.in)
		if ok != c.ok || got != c.norm {
			t.Errorf("pathToken(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.norm, c.ok)
		}
	}
}

func TestClassifyConst(t *testing.T) {
	cases := map[string]string{
		"claude-opus-4-8":         KindModel,
		"computer-use-2024-10-22": KindBeta,
		"page_location":           KindConst,
		"model_context_window":    KindConst,
	}
	for in, want := range cases {
		if got := classifyConst(in); got != want {
			t.Errorf("classifyConst(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddTagTokens(t *testing.T) {
	set := newTokenSet()
	addTagTokens("`json:\"prop,omitempty\" default:\"the_type\"`", set)
	if !set.has("prop") {
		t.Error("expected json prop")
	}
	if !set.has("the_type") || set.kindOf("the_type") != KindEnum {
		t.Error("expected default tag value as enum")
	}
}
