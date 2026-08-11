package modeladapter

import "testing"

func TestOpenAIEffectiveReasoningEffort(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		effort string
		want   string
	}{
		{"normal model keeps effort", "gpt-5", "high", "high"},
		{"grok-composer strips", "grok-composer-2.5", "high", ""},
		{"grok-composer-fast strips", "grok-composer-2.5-fast", "max", ""},
		{"composer prefix strips", "composer-x", "low", ""},
		{"empty effort stays empty", "gpt-5", "", ""},
		{"empty model keeps effort", "", "high", "high"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := openAIEffectiveReasoningEffort(c.model, c.effort); got != c.want {
				t.Errorf("openAIEffectiveReasoningEffort(%q, %q) = %q, want %q", c.model, c.effort, got, c.want)
			}
		})
	}
}

func TestStripOpenAIReasoningForUnsupportedModel(t *testing.T) {
	rejecting := map[string]any{"reasoning_effort": "high", "reasoning": map[string]any{"effort": "high"}, "model": "x"}
	stripOpenAIReasoningForUnsupportedModel(rejecting, "grok-composer-2.5")
	if _, ok := rejecting["reasoning_effort"]; ok {
		t.Error("expected reasoning_effort stripped for composer model")
	}
	if _, ok := rejecting["reasoning"]; ok {
		t.Error("expected reasoning stripped for composer model")
	}

	keeping := map[string]any{"reasoning_effort": "high"}
	stripOpenAIReasoningForUnsupportedModel(keeping, "gpt-5")
	if _, ok := keeping["reasoning_effort"]; !ok {
		t.Error("expected reasoning_effort kept for normal model")
	}
}
