package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cursor/internal/netproxy"
)

const modelListTimeout = 15 * time.Second

// ModelInfo 表示一条可用模型信息。
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// FetchModelListRequest 表示获取模型列表的请求参数。
type FetchModelListRequest struct {
	Type    string `json:"type"`    // "openai" | "anthropic"
	BaseURL string `json:"baseURL"` // OpenAI 兼容端点；Anthropic 固定官方地址
	APIKey  string `json:"apiKey"`
}

// FetchModelListResult 表示获取模型列表的结果。
type FetchModelListResult struct {
	Models []ModelInfo `json:"models"`
	Error  string      `json:"error"`
}

// FetchModelList 调用目标供应商的模型列表接口，返回可用模型列表。
func FetchModelList(req FetchModelListRequest) FetchModelListResult {
	ctx, cancel := context.WithTimeout(context.Background(), modelListTimeout)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(req.Type)) {
	case "anthropic":
		return fetchAnthropicModels(ctx, strings.TrimSpace(req.BaseURL), strings.TrimSpace(req.APIKey))
	default:
		return fetchOpenAIModels(ctx, strings.TrimSpace(req.BaseURL), strings.TrimSpace(req.APIKey))
	}
}

// fetchOpenAIModels 调用 OpenAI 兼容的 GET /v1/models 接口。
func fetchOpenAIModels(ctx context.Context, baseURL, apiKey string) FetchModelListResult {
	if baseURL == "" {
		return FetchModelListResult{Error: "接口地址不能为空"}
	}
	if apiKey == "" {
		return FetchModelListResult{Error: "访问密钥不能为空"}
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return FetchModelListResult{Error: fmt.Sprintf("构建请求失败: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := netproxy.NewHTTPClient(modelListTimeout).Do(req)
	if err != nil {
		return FetchModelListResult{Error: fmt.Sprintf("请求失败: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode != http.StatusOK {
		return FetchModelListResult{Error: fmt.Sprintf("接口返回 %d: %s", resp.StatusCode, truncate(string(body), 200))}
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return FetchModelListResult{Error: fmt.Sprintf("解析响应失败: %v", err)}
	}
	models := make([]ModelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" {
			models = append(models, ModelInfo{ID: id, DisplayName: id})
		}
	}
	if len(models) == 0 {
		return FetchModelListResult{Error: "未返回任何模型，请检查接口地址和密钥"}
	}
	return FetchModelListResult{Models: models}
}

// fetchAnthropicModels 调用 Anthropic GET /v1/models 接口。
// baseURL 为空时回退到官方地址，支持 Anthropic 协议中转站自定义地址。
func fetchAnthropicModels(ctx context.Context, baseURL, apiKey string) FetchModelListResult {
	if apiKey == "" {
		return FetchModelListResult{Error: "访问密钥不能为空"}
	}
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return FetchModelListResult{Error: fmt.Sprintf("构建请求失败: %v", err)}
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")

	resp, err := netproxy.NewHTTPClient(modelListTimeout).Do(req)
	if err != nil {
		return FetchModelListResult{Error: fmt.Sprintf("请求失败: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode != http.StatusOK {
		return FetchModelListResult{Error: fmt.Sprintf("接口返回 %d: %s", resp.StatusCode, truncate(string(body), 200))}
	}

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return FetchModelListResult{Error: fmt.Sprintf("解析响应失败: %v", err)}
	}
	models := make([]ModelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(m.DisplayName)
		if name == "" {
			name = id
		}
		models = append(models, ModelInfo{ID: id, DisplayName: name})
	}
	if len(models) == 0 {
		return FetchModelListResult{Error: "未返回任何模型，请检查密钥"}
	}
	return FetchModelListResult{Models: models}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
