// http_error.go 负责把非 2xx HTTP 响应整理成带响应体摘要的错误。
package modeladapter

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// maxErrorBodyBytes 表示错误响应体最多读取的字节数。
	maxErrorBodyBytes = 8192
)

// buildHTTPStatusError 读取响应体摘要并生成带状态码的错误。
func buildHTTPStatusError(prefix string, resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("%s response is nil", strings.TrimSpace(prefix))
	}

	limitedBody, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		if retrySummary := ProviderRetryAttemptSummary(resp); retrySummary != "" {
			return fmt.Errorf("%s status=%d %s body_read_error=%v", strings.TrimSpace(prefix), resp.StatusCode, retrySummary, err)
		}
		return fmt.Errorf("%s status=%d body_read_error=%v", strings.TrimSpace(prefix), resp.StatusCode, err)
	}
	retrySummary := ProviderRetryAttemptSummary(resp)
	bodyText := strings.TrimSpace(string(limitedBody))
	if bodyText == "" {
		if retrySummary != "" {
			return fmt.Errorf("%s status=%d %s", strings.TrimSpace(prefix), resp.StatusCode, retrySummary)
		}
		return fmt.Errorf("%s status=%d", strings.TrimSpace(prefix), resp.StatusCode)
	}
	if retrySummary != "" {
		return fmt.Errorf("%s status=%d %s body=%s", strings.TrimSpace(prefix), resp.StatusCode, retrySummary, bodyText)
	}
	return fmt.Errorf("%s status=%d body=%s", strings.TrimSpace(prefix), resp.StatusCode, bodyText)
}

// isHTTPStatusError 判断 error 是否来自 buildHTTPStatusError 且状态码匹配。
func isHTTPStatusError(err error, statusCode int) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), fmt.Sprintf("status=%d", statusCode))
}

// extractProviderErrorMessage 从 provider 错误中提取可读消息（取 body= 之后的响应体文本）。
func extractProviderErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if i := strings.Index(msg, "body="); i >= 0 {
		msg = strings.TrimSpace(msg[i+len("body="):])
	}
	return msg
}

// mapTechnicalProviderMessage 把常见技术错误映射为可读提示；未命中则回退到响应体文本。
func mapTechnicalProviderMessage(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case isHTTPStatusError(err, http.StatusTooManyRequests), isCapacityStyleProviderMessage(err):
		return "上游繁忙或限流，请稍后重试"
	case isHTTPStatusError(err, http.StatusUnauthorized):
		return "API Key 无效或未授权"
	case isHTTPStatusError(err, http.StatusForbidden):
		return "无权限访问该模型"
	case isHTTPStatusError(err, http.StatusNotFound):
		return "模型或端点不存在（404）"
	}
	return extractProviderErrorMessage(err)
}
