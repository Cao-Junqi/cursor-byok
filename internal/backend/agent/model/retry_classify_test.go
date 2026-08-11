package modeladapter

import (
	"errors"
	"net"
	"testing"
)

type testNetTimeoutError struct{}

func (testNetTimeoutError) Error() string   { return "read: i/o timeout (net)" }
func (testNetTimeoutError) Timeout() bool   { return true }
func (testNetTimeoutError) Temporary() bool { return false }

func TestIsTransientProviderStreamError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"status 429", errors.New("openai adapter status=429 body=too many requests"), true},
		{"status 502", errors.New("openai adapter status=502 body=bad gateway"), true},
		{"status 503", errors.New("openai adapter status=503 body=overloaded"), true},
		{"status 504", errors.New("openai adapter status=504 body=upstream timeout"), true},
		{"connection reset", errors.New("post: read tcp: connection reset by peer"), true},
		{"unexpected eof", errors.New("stream ended with unexpected eof"), true},
		{"service unavailable", errors.New("upstream service unavailable"), true},
		{"net timeout", testNetTimeoutError{}, true},
		{"unauthorized", errors.New("openai adapter status=401 body=unauthorized"), false},
		{"forbidden", errors.New("openai adapter status=403 body=forbidden"), false},
		{"积分不足", errors.New("openai adapter status=402 body=积分不足"), false},
		{"bad request", errors.New("openai adapter status=400 body=bad request"), false},
		{"context canceled", contextCanceledErr(), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTransientProviderStreamError(c.err); got != c.want {
				t.Errorf("isTransientProviderStreamError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestIsTransientProviderDialError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"connection reset", errors.New("read tcp: connection reset"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"no such host", errors.New("dial tcp: lookup no such host"), true},
		{"net timeout", testNetTimeoutError{}, true},
		{"bad request body", errors.New("status=400 body=bad request"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTransientProviderDialError(c.err); got != c.want {
				t.Errorf("isTransientProviderDialError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestIsCapacityStyleProviderMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"overloaded", errors.New("upstream overloaded"), true},
		{"pool exhausted", errors.New("no ls instance pool exhausted"), true},
		{"capacity", errors.New("no available capacity"), true},
		{"模型繁忙", errors.New("模型繁忙，请稍后"), true},
		{"请求过于频繁", errors.New("请求过于频繁"), true},
		{"connection reset", errors.New("connection reset"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isCapacityStyleProviderMessage(c.err); got != c.want {
				t.Errorf("isCapacityStyleProviderMessage(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func contextCanceledErr() error {
	return &testContextError{}
}

type testContextError struct{}

func (testContextError) Error() string { return "context canceled" }
func (testContextError) Timeout() bool { return false }

var _ net.Error = testNetTimeoutError{}
