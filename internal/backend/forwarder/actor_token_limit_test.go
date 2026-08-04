package forwarder

import (
	"encoding/json"
	"strings"
	"testing"

	"cursor/gen/agentv1"
)

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
		want              providerCompletionRecoveryDisposition
	}{
		{name: "responses completed reasoning only", finishReason: "completed", reasoning: "run tsc next", want: providerCompletionRecoveryResume},
		{name: "chat stop reasoning only", finishReason: "stop", reasoning: "run tsc next", want: providerCompletionRecoveryResume},
		{name: "anthropic message stop reasoning only", finishReason: "message_stop", reasoning: "run tsc next", want: providerCompletionRecoveryResume},
		{name: "visible response", finishReason: "completed", accumulatedText: "done", reasoning: "finished", want: providerCompletionRecoveryNone},
		{name: "tool invocation", finishReason: "completed", reasoning: "run tsc", hadToolInvocation: true, want: providerCompletionRecoveryNone},
		{name: "no reasoning", finishReason: "completed", want: providerCompletionRecoveryNone},
		{name: "token limit handled elsewhere", finishReason: "max_output_tokens", reasoning: "run tsc", want: providerCompletionRecoveryNone},
		{name: "second empty completion fails", finishReason: "completed", reasoning: "run tsc again", alreadyRecovered: true, want: providerCompletionRecoveryFail},
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

func TestIsBareContinuationRequest(t *testing.T) {
	for _, text := range []string{"继续", "继续。", "接着做", "continue", "Please continue!", "go on"} {
		if !isBareContinuationRequest(text) {
			t.Errorf("isBareContinuationRequest(%q) = false, want true", text)
		}
	}
	for _, text := range []string{"继续解释原因", "continue the explanation", "review", ""} {
		if isBareContinuationRequest(text) {
			t.Errorf("isBareContinuationRequest(%q) = true, want false", text)
		}
	}
}

func TestContinuationReminderOnlyAppliesInAgentMode(t *testing.T) {
	const marker = "short continuation message"
	tests := []struct {
		name           string
		mode           agentv1.AgentMode
		latestUserText string
		wantReminder   bool
	}{
		{name: "agent continuation", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", wantReminder: true},
		{name: "agent specific follow up", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续解释原因"},
		{name: "ask continuation", mode: agentv1.AgentMode_AGENT_MODE_ASK, latestUserText: "继续"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reminders := (&DefaultReminderInjector{}).Inject(test.mode, nil, nil, test.latestUserText, nil)
			found := false
			for _, reminder := range reminders.SystemParts {
				if strings.Contains(reminder, marker) {
					found = true
					break
				}
			}
			if found != test.wantReminder {
				t.Fatalf("continuation reminder present = %t, want %t", found, test.wantReminder)
			}
		})
	}
}

func TestClassifyContinuationWithoutAction(t *testing.T) {
	tests := []struct {
		name               string
		mode               agentv1.AgentMode
		latestUserText     string
		finishReason       string
		accumulatedText    string
		reasoning          string
		hadToolInvocation  bool
		alreadyRecovered   bool
		hadToolResult      bool
		hasIncompleteTodos bool
		recoveryAttempts   int
		want               providerCompletionRecoveryDisposition
	}{
		{name: "agent continuation progress only", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "Now simplify the flow.", reasoning: "Let me fix this.", want: providerCompletionRecoveryResume},
		{name: "second progress only fails", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "I will fix it.", reasoning: "Let me fix this.", alreadyRecovered: true, want: providerCompletionRecoveryFail},
		{name: "final response after tool result", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "Fixed and verified.", reasoning: "Work is complete.", alreadyRecovered: true, hadToolResult: true, want: providerCompletionRecoveryNone},
		{name: "historical tool result without recovery progress fails", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "Now update the login page.", reasoning: "I should continue.", alreadyRecovered: true, hadToolResult: true, recoveryAttempts: 1, want: providerCompletionRecoveryFail},
		{name: "unfinished todo after tool result resumes", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "Now update the login page.", reasoning: "The route is updated.", alreadyRecovered: true, hadToolResult: true, hasIncompleteTodos: true, want: providerCompletionRecoveryResume},
		{name: "unfinished todo without progress after recovery fails", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "Now update the login page.", reasoning: "I should continue.", alreadyRecovered: true, hadToolResult: true, hasIncompleteTodos: true, recoveryAttempts: 1, want: providerCompletionRecoveryFail},
		{name: "ask mode", mode: agentv1.AgentMode_AGENT_MODE_ASK, latestUserText: "继续", finishReason: "completed", accumulatedText: "More detail.", reasoning: "Continue answer.", want: providerCompletionRecoveryNone},
		{name: "specific follow up", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续解释原因", finishReason: "completed", accumulatedText: "Reason explained.", reasoning: "Explain.", want: providerCompletionRecoveryNone},
		{name: "tool invoked", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "Running tool.", reasoning: "Fix it.", hadToolInvocation: true, want: providerCompletionRecoveryNone},
		{name: "no reasoning", mode: agentv1.AgentMode_AGENT_MODE_AGENT, latestUserText: "继续", finishReason: "completed", accumulatedText: "Already complete.", want: providerCompletionRecoveryNone},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyContinuationWithoutAction(test.mode, test.latestUserText, test.finishReason, test.accumulatedText, test.reasoning, test.hadToolInvocation, test.alreadyRecovered, test.hadToolResult, test.hasIncompleteTodos, test.recoveryAttempts)
			if got != test.want {
				t.Fatalf("classifyContinuationWithoutAction() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestConversationHasIncompleteTodos(t *testing.T) {
	tests := []struct {
		name  string
		todos []*agentv1.TodoItem
		want  bool
	}{
		{name: "no todos"},
		{name: "pending todo", todos: []*agentv1.TodoItem{{Id: "1", Status: agentv1.TodoStatus_TODO_STATUS_PENDING}}, want: true},
		{name: "in progress todo", todos: []*agentv1.TodoItem{{Id: "1", Status: agentv1.TodoStatus_TODO_STATUS_IN_PROGRESS}}, want: true},
		{name: "completed and cancelled todos", todos: []*agentv1.TodoItem{
			{Id: "1", Status: agentv1.TodoStatus_TODO_STATUS_COMPLETED},
			{Id: "2", Status: agentv1.TodoStatus_TODO_STATUS_CANCELLED},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := conversationHasIncompleteTodos(&ConversationFile{CurrentTodos: test.todos})
			if err != nil {
				t.Fatalf("conversationHasIncompleteTodos() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("conversationHasIncompleteTodos() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestConversationHasIncompleteTodosUsesStructuredCheckpointState(t *testing.T) {
	payload, err := json.Marshal(runtimeStateEntryPayload{
		Todos: []*agentv1.TodoItem{{Id: "active", Status: agentv1.TodoStatus_TODO_STATUS_IN_PROGRESS}},
	})
	if err != nil {
		t.Fatalf("marshal runtime state: %v", err)
	}

	conversation := &ConversationFile{
		CurrentTodos: []*agentv1.TodoItem{{Id: "stale", Status: agentv1.TodoStatus_TODO_STATUS_COMPLETED}},
		Entries:      []HistoryEntry{{Seq: 1, Kind: "runtime_state", Payload: payload}},
	}
	got, err := conversationHasIncompleteTodos(conversation)
	if err != nil {
		t.Fatalf("conversationHasIncompleteTodos() error = %v", err)
	}
	if !got {
		t.Fatal("conversationHasIncompleteTodos() = false, want true for active structured todo")
	}
}

func TestContinuationRecoveryAttemptsResetAfterToolInvocation(t *testing.T) {
	if got := continuationRecoveryAttemptsForProviderDone(1, true); got != 0 {
		t.Fatalf("continuationRecoveryAttemptsForProviderDone(tool) = %d, want 0", got)
	}
	if got := continuationRecoveryAttemptsForProviderDone(1, false); got != 1 {
		t.Fatalf("continuationRecoveryAttemptsForProviderDone(no tool) = %d, want 1", got)
	}
}

func TestAppendContinuationRecoveryContextOncePerTurn(t *testing.T) {
	service := &Service{}
	stream := &ActiveStream{CheckpointConversation: &ConversationFile{}}

	for range 2 {
		if err := service.appendContinuationRecoveryContext(stream, "conversation-1", 7, "request-1"); err != nil {
			t.Fatalf("appendContinuationRecoveryContext() error = %v", err)
		}
	}

	conversation, _, _, err := service.snapshotCheckpointConversation(stream)
	if err != nil {
		t.Fatalf("snapshotCheckpointConversation() error = %v", err)
	}
	if len(conversation.Entries) != 1 {
		t.Fatalf("recovery entry count = %d, want 1", len(conversation.Entries))
	}
	if !currentTurnHasPromptContextSource(conversation, 7, promptContextSourceContinuationRecovery) {
		t.Fatal("continuation recovery prompt context was not recorded")
	}
}
