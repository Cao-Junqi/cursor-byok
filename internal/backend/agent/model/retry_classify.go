// retry_classify.go 负责 provider 错误的分类（瞬时/容量/不可重试）与退避。
// 语义移植自 CursorUltra 5.0.12（agent/model 层），子串表从二进制反汇编解码。
package modeladapter

import (
	"errors"
	"net"
	"strings"
	"time"
)

// maxProviderRequestRetries 是模型层 provider 请求的最大重试次数（共 1+3=4 次尝试）。
const maxProviderRequestRetries = 3

// transientStreamErrorSubstrings 对应 ultra isTransientProviderStreamError 的子串表。
var transientStreamErrorSubstrings = []string{
	"暂时不可用", "暂时异常",
	"stream error", "internal_error", "received from peer",
	"http2", "connection reset",
	"status=429", "status=502", "status=503", "status=504",
	"bad gateway", "service unavailable",
}

// transientDialErrorSubstrings 对应 ultra isTransientProviderDialError 的子串表。
var transientDialErrorSubstrings = []string{
	"connection refused", "connection reset", "broken pipe", "i/o timeout",
	"tls handshake timeout", "no such host", "temporary failure",
	"server closed idle connection", "unexpected eof", "eof",
	"stream error", "internal_error", "http2",
}

// nonRetryableProductErrorSubstrings 对应 ultra isNonRetryableProductError 的子串表。
var nonRetryableProductErrorSubstrings = []string{
	"积分不足", "insufficient points", "授权码", "账号池",
	"forbidden", "授权校验失败", "method not allowed",
	"authorization rejected", "request body too large", "unauthorized",
	"response.incomplete",
}

// capacityStyleMessageSubstrings 对应 ultra isCapacityStyleProviderMessage 的子串表。
var capacityStyleMessageSubstrings = []string{
	"capacity", "overloaded", "pool exhausted", "pool_exhausted",
	"no active accounts", "no ls instance", "temporarily unavailable",
	"service unavailable", "模型高峰", "模型繁忙", "高峰期",
	"没有可用容量", "没有可用账号", "请求过于频繁",
	"暂时没有可用授权", "账号队列超时",
}

// isTransientProviderDialError 判断错误是否为瞬时 dial/连接层错误（net.Error 或已知瞬时子串）。
func isTransientProviderDialError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return containsAnyFold(err.Error(), transientDialErrorSubstrings)
}

// isNonRetryableProductError 判断错误是否为明确不可重试的产品/授权错误。
func isNonRetryableProductError(err error) bool {
	if err == nil {
		return false
	}
	return containsAnyFold(err.Error(), nonRetryableProductErrorSubstrings)
}

// isCapacityStyleProviderMessage 判断错误是否为容量/限流类（触发更长退避）。
func isCapacityStyleProviderMessage(err error) bool {
	if err == nil {
		return false
	}
	return containsAnyFold(err.Error(), capacityStyleMessageSubstrings)
}

// isTransientProviderStreamError 判断 provider 流错误是否瞬时可重试。
// 排除不可重试产品错误；命中瞬时子串（含 status=429/502/503/504）为真；兜底走 dial 分类。
func isTransientProviderStreamError(err error) bool {
	if err == nil {
		return false
	}
	if isNonRetryableProductError(err) {
		return false
	}
	if containsAnyFold(err.Error(), transientStreamErrorSubstrings) {
		return true
	}
	return isTransientProviderDialError(err)
}

// sleepProviderRetry 普通瞬时错误退避：200ms×2ⁿ，封顶 800ms（对齐 ultra）。
func sleepProviderRetry(attempt int) {
	delay := 200 * time.Millisecond
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	if delay > 800*time.Millisecond {
		delay = 800 * time.Millisecond
	}
	time.Sleep(delay)
}

// sleepProviderCapacityRetry 容量错误退避：0→800ms、1→1.5s、≥2→2s（对齐 ultra）。
func sleepProviderCapacityRetry(attempt int) {
	var delay time.Duration
	switch {
	case attempt <= 0:
		delay = 800 * time.Millisecond
	case attempt == 1:
		delay = 1500 * time.Millisecond
	default:
		delay = 2 * time.Second
	}
	time.Sleep(delay)
}

// containsAnyFold 小写化后检查 msg 是否包含任一子串。
func containsAnyFold(msg string, subs []string) bool {
	lower := strings.ToLower(msg)
	for _, sub := range subs {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}
