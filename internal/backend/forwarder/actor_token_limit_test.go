package forwarder

import "testing"

func TestShouldResumeProviderAfterDone(t *testing.T) {
	tests := []struct {
		name                   string
		finishReason           string
		hadToolInvocation      bool
		terminalToolInvocation bool
		want                   bool
	}{
		{name: "normal stop", finishReason: "stop", want: false},
		{name: "completed tool", finishReason: "stop", hadToolInvocation: true, want: true},
		{name: "provider tool reason", finishReason: "tool_calls", want: true},
		{name: "token limit", finishReason: "length", want: true},
		{name: "normalized token limit", finishReason: " LENGTH ", want: true},
		{name: "responses token limit", finishReason: "max_output_tokens", want: true},
		{name: "anthropic token limit", finishReason: "max_tokens", want: true},
		{name: "incomplete response", finishReason: "incomplete", want: true},
		{name: "terminal tool", finishReason: "length", terminalToolInvocation: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := shouldResumeProviderAfterDone(test.finishReason, test.hadToolInvocation, test.terminalToolInvocation)
			if got != test.want {
				t.Fatalf("shouldResumeProviderAfterDone(%q, %t, %t) = %t, want %t", test.finishReason, test.hadToolInvocation, test.terminalToolInvocation, got, test.want)
			}
		})
	}
}

func TestIsTokenLimitFinishReason(t *testing.T) {
	for _, reason := range []string{"length", "max_tokens", "max_output_tokens", "max_completion_tokens", "token_limit", "token_limit_exceeded", "incomplete"} {
		if !isTokenLimitFinishReason(reason) {
			t.Errorf("isTokenLimitFinishReason(%q) = false, want true", reason)
		}
	}
	for _, reason := range []string{"stop", "tool_calls", "content_filter", "error", ""} {
		if isTokenLimitFinishReason(reason) {
			t.Errorf("isTokenLimitFinishReason(%q) = true, want false", reason)
		}
	}
}

func TestAppendTokenLimitRecoveryContextOncePerTurn(t *testing.T) {
	service := &Service{}
	stream := &ActiveStream{CheckpointConversation: &ConversationFile{}}

	for range 2 {
		if err := service.appendTokenLimitRecoveryContext(stream, "conversation-1", 7, "request-1"); err != nil {
			t.Fatalf("appendTokenLimitRecoveryContext() error = %v", err)
		}
	}

	conversation, _, _, err := service.snapshotCheckpointConversation(stream)
	if err != nil {
		t.Fatalf("snapshotCheckpointConversation() error = %v", err)
	}
	if len(conversation.Entries) != 1 {
		t.Fatalf("recovery entry count = %d, want 1", len(conversation.Entries))
	}
	if !currentTurnHasPromptContextSource(conversation, 7, promptContextSourceTokenLimitRecovery) {
		t.Fatal("token-limit recovery prompt context was not recorded")
	}
}
