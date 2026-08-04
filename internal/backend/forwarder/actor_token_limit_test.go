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

func TestClassifyEmptyProviderCompletion(t *testing.T) {
	tests := []struct {
		name              string
		finishReason      string
		accumulatedText   string
		reasoning         string
		hadToolInvocation bool
		alreadyRecovered  bool
		want              emptyCompletionRecoveryDisposition
	}{
		{name: "responses completed reasoning only", finishReason: "completed", reasoning: "run tsc next", want: emptyCompletionRecoveryResume},
		{name: "chat stop reasoning only", finishReason: "stop", reasoning: "run tsc next", want: emptyCompletionRecoveryResume},
		{name: "anthropic message stop reasoning only", finishReason: "message_stop", reasoning: "run tsc next", want: emptyCompletionRecoveryResume},
		{name: "visible response", finishReason: "completed", accumulatedText: "done", reasoning: "finished", want: emptyCompletionRecoveryNone},
		{name: "tool invocation", finishReason: "completed", reasoning: "run tsc", hadToolInvocation: true, want: emptyCompletionRecoveryNone},
		{name: "no reasoning", finishReason: "completed", want: emptyCompletionRecoveryNone},
		{name: "token limit handled elsewhere", finishReason: "max_output_tokens", reasoning: "run tsc", want: emptyCompletionRecoveryNone},
		{name: "second empty completion fails", finishReason: "completed", reasoning: "run tsc again", alreadyRecovered: true, want: emptyCompletionRecoveryFail},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyEmptyProviderCompletion(test.finishReason, test.accumulatedText, test.reasoning, test.hadToolInvocation, test.alreadyRecovered)
			if got != test.want {
				t.Fatalf("classifyEmptyProviderCompletion() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestAppendEmptyCompletionRecoveryContextOncePerTurn(t *testing.T) {
	service := &Service{}
	stream := &ActiveStream{CheckpointConversation: &ConversationFile{}}

	for range 2 {
		if err := service.appendEmptyCompletionRecoveryContext(stream, "conversation-1", 7, "request-1"); err != nil {
			t.Fatalf("appendEmptyCompletionRecoveryContext() error = %v", err)
		}
	}

	conversation, _, _, err := service.snapshotCheckpointConversation(stream)
	if err != nil {
		t.Fatalf("snapshotCheckpointConversation() error = %v", err)
	}
	if len(conversation.Entries) != 1 {
		t.Fatalf("recovery entry count = %d, want 1", len(conversation.Entries))
	}
	if !currentTurnHasPromptContextSource(conversation, 7, promptContextSourceEmptyCompletionRecovery) {
		t.Fatal("empty-completion recovery prompt context was not recorded")
	}
}
