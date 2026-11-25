package respconv

import (
	"net/http/httptest"
	"testing"

	"github.com/d-kuro/kirocc/internal/kiroproto"
)

func TestStopDiagnostics(t *testing.T) {
	reasoning := kiroproto.Event{Type: kiroproto.EventReasoningContent, ThinkingText: "thinking"}
	malformed := kiroproto.Event{Type: kiroproto.EventMetadata, UpstreamStopReason: "MALFORMED_TOOL_USE"}
	for _, tc := range []struct {
		name                      string
		events                    []kiroproto.Event
		shape, upstream, resolved string
		retry                     bool
	}{
		{"no events", nil, "empty", "absent", "end_turn", false},
		{"malformed followed by usage", []kiroproto.Event{reasoning, malformed, {Type: kiroproto.EventMetadata, OutputTokens: 2}}, "reasoning_only", "MALFORMED_TOOL_USE", "end_turn", true},
		{"redacted only", []kiroproto.Event{{Type: kiroproto.EventReasoningContent, RedactedContent: "blob"}}, "reasoning_only", "absent", "end_turn", true},
		{"visible text", []kiroproto.Event{reasoning, {Type: kiroproto.EventAssistantResponse, Content: "answer"}, malformed}, "visible", "MALFORMED_TOOL_USE", "end_turn", false},
		{"visible tool", []kiroproto.Event{{Type: kiroproto.EventToolUse, ToolStop: true, ToolName: "read", ToolUseID: "id", ToolInput: "{}"}}, "visible", "absent", "tool_use", false},
		{"local stop", []kiroproto.Event{reasoning, malformed, {Type: kiroproto.EventAssistantResponse, Content: "STOP"}}, "reasoning_only", "MALFORMED_TOOL_USE", "stop_sequence", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSSEWriter(t.Context(), httptest.NewRecorder(), "model", 0, []string{"STOP"}, 0, 0)
			n := NewNonStreamingAccumulator(0, []string{"STOP"}, 0, 0)
			for _, e := range tc.events {
				s.HandleEvent(e)
				n.ProcessEvent(e)
			}
			if err := s.Finish(); err != nil {
				t.Fatal(err)
			}
			response, _ := n.BuildResponse("model")
			if response["stop_reason"] != tc.resolved {
				t.Errorf("client stop_reason = %v, want %s", response["stop_reason"], tc.resolved)
			}
			for _, sink := range []interface {
				StopDiagnostics() (string, string, string)
				IsEmptyVisibleEndTurn() bool
			}{s, n} {
				shape, upstream, resolved := sink.StopDiagnostics()
				if shape != tc.shape || upstream != tc.upstream || resolved != tc.resolved {
					t.Errorf("%T diagnostics = (%q, %q, %q), want (%q, %q, %q)", sink, shape, upstream, resolved, tc.shape, tc.upstream, tc.resolved)
				}
				if got := sink.IsEmptyVisibleEndTurn(); got != tc.retry {
					t.Errorf("%T IsEmptyVisibleEndTurn() = %v, want %v", sink, got, tc.retry)
				}
			}
		})
	}
}
