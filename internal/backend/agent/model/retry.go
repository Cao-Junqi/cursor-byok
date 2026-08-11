// retry.go 负责 provider HTTP 请求的服务端重试：
// 传输错误与 HTTP 状态错误（429/502/503/504 等瞬时错误）在零内容投递前自动重试，
// 容量/限流错误走更长退避。语义对齐 CursorUltra 5.0.12 的 agent/model 层。
package modeladapter

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

// providerRetryHeader 标记最终响应经过了多少次重试，供 ProviderRetryAttemptSummary 读取。
const providerRetryHeader = "X-Provider-Retries"

// DoProviderRequestWithRetry 发送 provider HTTP 请求并做服务端重试（旧入口名保留）。
func DoProviderRequestWithRetry(
	ctx context.Context,
	client *http.Client,
	provider string,
	requestID string,
	modelCallID string,
	buildRequest func(context.Context) (*http.Request, error),
) (*http.Response, error) {
	return doProviderRequestWithRetry(ctx, client, provider, requestID, modelCallID, buildRequest)
}

func doProviderRequestWithRetry(
	ctx context.Context,
	client *http.Client,
	provider string,
	requestID string,
	modelCallID string,
	buildRequest func(context.Context) (*http.Request, error),
) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	var lastErr error
	retries := 0 // 已执行的重试次数
	for attempt := 0; attempt <= maxProviderRequestRetries; attempt++ {
		if attempt > 0 {
			if isCapacityStyleProviderMessage(lastErr) {
				sleepProviderCapacityRetry(attempt)
			} else {
				sleepProviderRetry(attempt)
			}
		}

		httpReq, err := buildRequest(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if !isTransientProviderStreamError(err) {
				return nil, err
			}
			retries++
			lastErr = err
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if retries > 0 {
				resp.Header.Set(providerRetryHeader, strconv.Itoa(retries))
			}
			return resp, nil
		}
		// 非 2xx：读取状态错误并分类；瞬时（429/5xx）重试，否则立即返回。
		if retries > 0 {
			resp.Header.Set(providerRetryHeader, strconv.Itoa(retries))
		}
		statusErr := buildHTTPStatusError(provider+" adapter", resp)
		_ = resp.Body.Close()
		if !isTransientProviderStreamError(statusErr) {
			return nil, statusErr
		}
		retries++
		lastErr = statusErr
		continue
	}
	if mapped := mapTechnicalProviderMessage(lastErr); mapped != "" {
		return nil, fmt.Errorf("%w (retried %d times; %s)", lastErr, retries, mapped)
	}
	return nil, fmt.Errorf("%w (retried %d times)", lastErr, retries)
}

// ProviderRetryAttemptSummary 返回最终响应经过的重试次数摘要（无重试则空）。
func ProviderRetryAttemptSummary(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	if v := resp.Header.Get(providerRetryHeader); v != "" {
		return "retries=" + v
	}
	return ""
}
