// Command driftcheck detects when this hand-written Anthropic SDK's types have
// drifted out of sync with the real Anthropic API.
//
// It extracts wire-level tokens (JSON property names, enum/const string values,
// model IDs, anthropic-beta header strings, tool type strings and endpoint URL
// paths) from this repo's Go source, extracts the same token set from an
// authoritative ground truth (the spec-generated github.com/anthropics/anthropic-
// sdk-go), and reports every ground-truth token we are missing. An allowlist file
// suppresses intentional omissions so the check can be made clean.
//
// Exit codes: 0 = no drift, 1 = drift found, 2 = operational error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type options struct {
	oursDir   string
	allowlist string
	sdkDir    string
	sdkRepo   string
	sdkRef    string
	files     string
	jsonOut   bool
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("driftcheck", flag.ContinueOnError)
	var o options
	fs.StringVar(&o.oursDir, "ours", ".", "path to this SDK's repo root")
	fs.StringVar(&o.allowlist, "allowlist", "cmd/driftcheck/allowlist.txt",
		"path to allowlist file (intentional omissions)")
	fs.StringVar(&o.sdkDir, "sdk-dir", envOr("DRIFTCHECK_SDK_DIR", ""),
		"use an already-checked-out anthropic-sdk-go dir instead of cloning")
	fs.StringVar(&o.sdkRepo, "sdk-repo", envOr("DRIFTCHECK_SDK_REPO", defaultSDKRepo),
		"git URL of the ground-truth SDK")
	fs.StringVar(&o.sdkRef, "sdk-ref", envOr("DRIFTCHECK_SDK_REF", "main"),
		"git ref (branch, tag or commit) of the ground-truth SDK")
	fs.StringVar(&o.files, "files", "",
		"comma-separated SDK-relative files to scan (default: core message/batch/model surface)")
	fs.BoolVar(&o.jsonOut, "json", false, "emit machine-readable JSON instead of a text report")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	return o, nil
}

type report struct {
	Source      string  `json:"source"`
	OursTokens  int     `json:"ours_tokens"`
	TruthTokens int     `json:"truth_tokens"`
	Allowlist   int     `json:"allowlisted"`
	DriftCount  int     `json:"drift_count"`
	Drift       []Token `json:"drift"`
}

func main() {
	os.Exit(run2(os.Args[1:], os.Stdout, os.Stderr))
}

func run2(args []string, stdout, stderr *os.File) int {
	o, err := parseFlags(args)
	if err != nil {
		return 2
	}

	ours, err := extractDir(o.oursDir, []string{"driftcheck", "testdata", ".git", "vendor"})
	if err != nil {
		fmt.Fprintf(stderr, "driftcheck: failed to scan this repo: %v\n", err)
		return 2
	}

	var files []string
	if o.files != "" {
		files = splitComma(o.files)
	}
	truth, source, err := loadGroundTruth(groundTruthOptions{
		dir:   o.sdkDir,
		repo:  o.sdkRepo,
		ref:   o.sdkRef,
		files: files,
	})
	if err != nil {
		fmt.Fprintf(stderr, "driftcheck: failed to load ground truth: %v\n", err)
		return 2
	}

	allow, err := loadAllowlist(o.allowlist)
	if err != nil {
		fmt.Fprintf(stderr, "driftcheck: failed to read allowlist: %v\n", err)
		return 2
	}

	drift := diff(ours, truth, allow)

	rep := report{
		Source:      source,
		OursTokens:  len(ours.m),
		TruthTokens: len(truth.m),
		Allowlist:   len(allow.values),
		DriftCount:  len(drift),
		Drift:       drift,
	}

	if o.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		writeTextReport(stdout, rep)
	}

	if len(drift) > 0 {
		return 1
	}
	return 0
}

func writeTextReport(w *os.File, rep report) {
	fmt.Fprintln(w, "driftcheck report")
	fmt.Fprintln(w, "=================")
	fmt.Fprintf(w, "ground truth : %s\n", rep.Source)
	fmt.Fprintf(w, "our tokens   : %d\n", rep.OursTokens)
	fmt.Fprintf(w, "truth tokens : %d\n", rep.TruthTokens)
	fmt.Fprintf(w, "allowlisted  : %d\n", rep.Allowlist)
	fmt.Fprintf(w, "drift        : %d\n\n", rep.DriftCount)

	if rep.DriftCount == 0 {
		fmt.Fprintln(w, "No drift: this SDK is in sync with the ground truth (modulo allowlist).")
		return
	}

	kinds, byKind := groupByKind(rep.Drift)
	for _, k := range kinds {
		fmt.Fprintf(w, "%s (%d) — present in Anthropic API, missing here:\n",
			kindLabel(k), len(byKind[k]))
		for _, t := range byKind[k] {
			if t.Raw != "" && t.Raw != t.Value {
				fmt.Fprintf(w, "  - %s (raw: %s)\n", t.Value, t.Raw)
			} else {
				fmt.Fprintf(w, "  - %s\n", t.Value)
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w,
		"Add intentional omissions to the allowlist to silence them; fix real gaps in the SDK.")
}

func kindLabel(k string) string {
	switch k {
	case KindModel:
		return "MODEL IDs"
	case KindBeta:
		return "BETA HEADERS"
	case KindPath:
		return "ENDPOINT PATHS"
	case KindEnum:
		return "ENUM / TYPE VALUES"
	case KindJSON:
		return "JSON PROPERTIES"
	default:
		return "CONST STRINGS"
	}
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
