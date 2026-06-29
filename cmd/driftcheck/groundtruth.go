package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultSDKRepo is the official, spec-generated Anthropic Go SDK. Its types are
// produced from Anthropic's OpenAPI spec by Stainless, so its wire tokens are an
// authoritative ground truth.
const defaultSDKRepo = "https://github.com/anthropics/anthropic-sdk-go"

// defaultTruthFiles are the SDK source files whose surface this hand-written SDK
// actually mirrors (messages, batches, completions, models, beta versions, shared
// errors/enums). Restricting to these keeps the diff focused and low-noise; the
// huge beta-agent/session/vault surface is intentionally excluded.
var defaultTruthFiles = []string{
	"message.go",
	"messagebatch.go",
	"completion.go",
	"model.go",
	"beta.go",
	"shared/shared.go",
	"shared/constant/constants.go",
}

// groundTruthOptions configures where the ground truth comes from.
type groundTruthOptions struct {
	dir   string   // pre-checked-out SDK dir; if set, no fetch happens
	repo  string   // git URL to clone when dir is empty
	ref   string   // git ref to check out
	files []string // SDK-relative files to scan
}

// loadGroundTruth resolves the SDK source (using an existing dir or cloning it),
// extracts its tokens, and returns them plus a human-readable source description.
func loadGroundTruth(opts groundTruthOptions) (*TokenSet, string, error) {
	files := opts.files
	if len(files) == 0 {
		files = defaultTruthFiles
	}

	dir := opts.dir
	source := ""
	if dir != "" {
		source = fmt.Sprintf("local SDK checkout %s", dir)
	} else {
		tmp, err := fetchSDK(opts.repo, opts.ref)
		if err != nil {
			return nil, "", err
		}
		defer os.RemoveAll(tmp)
		dir = tmp
		source = fmt.Sprintf("%s@%s (anthropic-sdk-go, spec-generated)", opts.repo, opts.ref)
	}

	paths, err := resolveFiles(dir, files)
	if err != nil {
		return nil, "", err
	}
	set, err := extractFiles(paths)
	if err != nil {
		return nil, "", err
	}
	return set, source, nil
}

// resolveFiles maps SDK-relative file names to absolute paths, erroring if any
// are missing (a missing file usually means the SDK layout changed).
func resolveFiles(dir string, files []string) ([]string, error) {
	var out []string
	var missing []string
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if _, err := os.Stat(p); err != nil {
			missing = append(missing, f)
			continue
		}
		out = append(out, p)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"ground-truth files not found in SDK checkout (layout may have changed): %s",
			strings.Join(missing, ", "),
		)
	}
	return out, nil
}

// fetchSDK shallow-clones repo at ref into a temp dir. It first tries a fast
// branch/tag clone and falls back to a full clone + checkout for commit SHAs.
func fetchSDK(repo, ref string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found on PATH: %w", err)
	}
	if repo == "" {
		repo = defaultSDKRepo
	}
	if ref == "" {
		ref = "main"
	}

	tmp, err := os.MkdirTemp("", "driftcheck-sdk-")
	if err != nil {
		return "", err
	}

	// Fast path: shallow clone of a branch or tag.
	if out, err := run("git", "clone", "--depth", "1", "--branch", ref, repo, tmp); err == nil {
		return tmp, nil
	} else {
		// Fall back to a full clone then checkout (handles commit SHAs).
		_ = os.RemoveAll(tmp)
		if tmp, err = os.MkdirTemp("", "driftcheck-sdk-"); err != nil {
			return "", err
		}
		if out2, err2 := run("git", "clone", repo, tmp); err2 != nil {
			os.RemoveAll(tmp)
			return "", fmt.Errorf(
				"failed to clone %s (are you offline?): %v\n%s\n%s",
				repo, err2, out, out2,
			)
		}
		if out3, err3 := run("git", "-C", tmp, "checkout", ref); err3 != nil {
			os.RemoveAll(tmp)
			return "", fmt.Errorf("failed to checkout ref %q: %v\n%s", ref, err3, out3)
		}
	}
	return tmp, nil
}

func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
