package main

import (
	"bufio"
	"os"
	"sort"
	"strings"
)

// Allowlist holds wire values that are intentionally absent from this SDK
// (unsupported endpoints, legacy features, etc.) and should not be reported as
// drift.
type Allowlist struct {
	values map[string]bool
}

func newAllowlist() *Allowlist { return &Allowlist{values: map[string]bool{}} }

func (a *Allowlist) has(v string) bool { return a.values[v] }

// loadAllowlist reads an allowlist file. Each non-empty line is one wire value;
// "#" begins a comment (whole-line or trailing). A missing path yields an empty
// allowlist without error so the tool works before one is created.
func loadAllowlist(path string) (*Allowlist, error) {
	a := newAllowlist()
	if path == "" {
		return a, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return a, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		a.values[line] = true
	}
	return a, sc.Err()
}

// diff returns ground-truth tokens that are absent from ours and not allowlisted.
func diff(ours, truth *TokenSet, allow *Allowlist) []Token {
	var out []Token
	for _, t := range truth.sorted() {
		if ours.has(t.Value) || allow.has(t.Value) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// groupByKind buckets drift tokens by kind, returning kinds in display order.
func groupByKind(drift []Token) ([]string, map[string][]Token) {
	byKind := map[string][]Token{}
	for _, t := range drift {
		byKind[t.Kind] = append(byKind[t.Kind], t)
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kindPriority(kinds[i]) > kindPriority(kinds[j])
	})
	for _, k := range kinds {
		sort.Slice(byKind[k], func(i, j int) bool {
			return byKind[k][i].Value < byKind[k][j].Value
		})
	}
	return kinds, byKind
}
