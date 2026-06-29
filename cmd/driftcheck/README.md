# driftcheck

`driftcheck` detects when this hand-written Anthropic SDK's types have drifted
out of sync with the real Anthropic API (wrong JSON tags, missing fields, missing
enum values, stale model IDs, missing endpoints, missing beta headers).

## What it checks

The API contract is fundamentally a set of **wire-level tokens** — strings that go
on the wire and are therefore comparable across SDKs regardless of Go type names:

- **JSON property names** (from struct field tags)
- **Enum / discriminator type values** (string consts and `default:"…"` tags,
  e.g. `page_location`, `model_context_window_exceeded`, `web_search_20250305`)
- **Model ID strings** (e.g. `claude-opus-4-8`)
- **anthropic-beta header values** (e.g. `interleaved-thinking-2025-05-14`)
- **Endpoint URL paths** (normalized: leading `/` and `v1/` stripped, so
  `/messages` and `v1/messages` compare equal)

driftcheck extracts these tokens from this repo's `*.go` via `go/ast`, extracts
the same token set from a ground truth, and reports every ground-truth token this
SDK is missing.

## Ground truth

The ground truth is the official **[`github.com/anthropics/anthropic-sdk-go`]** —
its types are generated from Anthropic's OpenAPI spec by Stainless, so its wire
tokens are authoritative. We use it (rather than fetching the raw OpenAPI spec)
because there is no stable, documented public URL for the spec, whereas the SDK is
reliably fetchable via `git` and is the same spec in Go form.

By default driftcheck scans only the SDK source files whose surface this SDK
mirrors — messages, message batches, completions, models, beta versions and
shared errors/enums — to keep the diff focused and low-noise. The large
beta-agent / session / vault / skill surface is intentionally out of scope.

[`github.com/anthropics/anthropic-sdk-go`]: https://github.com/anthropics/anthropic-sdk-go

## Running

```sh
# Clone the ground-truth SDK (main) and check the repo in the working directory:
go run ./cmd/driftcheck

# See *everything* that diverges, ignoring the allowlist:
go run ./cmd/driftcheck -allowlist /dev/null

# Machine-readable output:
go run ./cmd/driftcheck -json

# Pin a specific SDK ref, or reuse a local checkout (no network):
go run ./cmd/driftcheck -sdk-ref v1.2.3
go run ./cmd/driftcheck -sdk-dir /path/to/anthropic-sdk-go
```

Exit codes: `0` no drift, `1` drift found, `2` operational error (e.g. offline
and unable to clone — the error is printed to stderr).

### Flags / env

| Flag | Env | Default | Meaning |
|------|-----|---------|---------|
| `-ours` | | `.` | This SDK's repo root |
| `-allowlist` | | `cmd/driftcheck/allowlist.txt` | Intentional-omissions file |
| `-sdk-dir` | `DRIFTCHECK_SDK_DIR` | _(clone)_ | Use an existing SDK checkout |
| `-sdk-repo` | `DRIFTCHECK_SDK_REPO` | anthropic-sdk-go | Ground-truth git URL |
| `-sdk-ref` | `DRIFTCHECK_SDK_REF` | `main` | Branch/tag/commit to fetch |
| `-files` | | core surface | Comma-separated SDK-relative files to scan |
| `-json` | | `false` | JSON output |

## The allowlist

`allowlist.txt` lists wire tokens the Anthropic SDK exposes but this SDK
**intentionally** does not support (unsupported tools, features and endpoints, or
dynamic-path false positives this SDK builds by string concatenation). One token
per line; `#` starts a comment.

It is a **baseline snapshot**: it captures everything that diverged at the time
the tool was added, so the check is green and only *new* divergences fail CI.

To update it:

- **A new feature was implemented here** → delete its tokens from the allowlist so
  future regressions on it are caught.
- **A new intentional omission appears** (CI now fails) → if it really is
  unsupported, append the reported tokens (run `-json` for exact values).
- **A real gap is reported** → fix the SDK type; do not allowlist it.

## CI

`.github/workflows/drift-check.yml` runs driftcheck on every pull request, weekly
on a cron, and on manual dispatch. The job fails when un-allowlisted drift exists
and always uploads the report as an artifact.

## Tests

`go test ./cmd/driftcheck/` runs fully offline against fixtures in `testdata/`.
The live `git`-fetch path is covered by a test gated behind `DRIFTCHECK_LIVE=1`.
