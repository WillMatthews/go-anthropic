// Package ours is a fixture standing in for this SDK. It deliberately omits some
// tokens present in the truth fixture so driftcheck has drift to find.
package ours

type Model string

// Missing ModelBeta ("claude-test-beta") on purpose -> should be reported.
const ModelAlpha Model = "claude-test-alpha"

type StopReason string

// Missing StopRefusal ("refusal") on purpose -> should be reported.
const StopEndTurn StopReason = "end_turn"

type Request struct {
	FieldOne string `json:"field_one"`
	// Missing FieldTwo ("field_two") and the "widget_block" type value.
}

func endpoints() {
	// Only the create path; "widgets/cancel" is missing.
	create := "/widgets"
	_ = create
}
