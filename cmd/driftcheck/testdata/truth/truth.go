// Package truth is a ground-truth fixture for driftcheck tests. It lives under
// testdata/ so the Go toolchain ignores it; driftcheck parses it as data.
package truth

type Model string

const (
	ModelAlpha Model = "claude-test-alpha"
	ModelBeta  Model = "claude-test-beta"
)

type BetaVersion string

const FeatureBeta BetaVersion = "widget-feature-2025-01-01"

type StopReason string

const (
	StopEndTurn StopReason = "end_turn"
	StopRefusal StopReason = "refusal"
)

type Request struct {
	FieldOne   string `json:"field_one"`
	FieldTwo   string `json:"field_two,omitempty"`
	Type       string `json:"type" default:"widget_block"`
	NotAField  string `json:"-"`
	unexported string
}

func endpoints() {
	create := "v1/widgets"
	get := "/widgets/cancel"
	_, _ = create, get
}
