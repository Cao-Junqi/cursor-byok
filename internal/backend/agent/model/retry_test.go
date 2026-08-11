package modeladapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cursor/internal/modelchannel"
)

// TestDoProviderRequestWithRetryTransient 验证瞬时 429 自动重试后成功，且响应带重试摘要头。
func TestDoProviderRequestWithRetryTransient(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limit"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	build := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader("{}"))
	}

	resp, err := doProviderRequestWithRetry(context.Background(), srv.Client(), "openai", "rid-1", "mid-1", build)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if calls != 2 {
		t.Errorf("expected 2 attempts (1 retry), got %d", calls)
	}
	if got := resp.Header.Get(providerRetryHeader); got != "1" {
		t.Errorf("retry header = %q, want 1", got)
	}
}

// TestDoProviderRequestWithRetryExhausts 验证连续瞬时错误重试耗尽后返回带提示的错误。
func TestDoProviderRequestWithRetryExhausts(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	build := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader("{}"))
	}

	resp, err := doProviderRequestWithRetry(context.Background(), srv.Client(), "openai", "rid-2", "mid-2", build)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	if calls != maxProviderRequestRetries+1 {
		t.Errorf("expected %d attempts, got %d", maxProviderRequestRetries+1, calls)
	}
	if !strings.Contains(err.Error(), "上游繁忙或限流") {
		t.Errorf("expected readable hint in error, got: %v", err)
	}
}

// TestDoProviderRequestWithRetryNonRetryable 验证不可重试错误（如 401）立即返回，不重试。
func TestDoProviderRequestWithRetryNonRetryable(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	build := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader("{}"))
	}

	_, err := doProviderRequestWithRetry(context.Background(), srv.Client(), "openai", "rid-3", "mid-3", build)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", calls)
	}
}

// TestOpenAIAdapterStreamRetriesTransient429 端到端验证：mock 上游首答 429，重试后正常流式，
// 内容正常投递且零重复。
func TestOpenAIAdapterStreamRetriesTransient429(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limit"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	adapter := NewOpenAIAdapter()
	req := StreamRequest{
		BaseURL:         srv.URL,
		APIKey:          "test-key",
		ProviderModelID: "test-model",
		OpenAIEndpoint:  modelchannel.OpenAIEndpointChatCompletions,
		Messages:        []Message{{Role: "user", Content: "hi"}},
		RequestID:       "e2e-1",
		ModelCallID:     "e2e-call-1",
	}

	var gotText strings.Builder
	err := adapter.Stream(context.Background(), req, func(ev ModelEvent) error {
		if ev.Kind == ModelEventKindTextDelta {
			gotText.WriteString(ev.Text)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", calls)
	}
	if got := gotText.String(); got != "Hello" {
		t.Errorf("got text %q, want Hello", got)
	}
}
