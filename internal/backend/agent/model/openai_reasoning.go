// openai_reasoning.go 处理 OpenAI 模型对 reasoning_effort 的兼容：
// 对拒绝该参数的模型（xAI grok-composer 系列等）自动剥离，避免 400/token 异常。
// 语义对齐 CursorUltra 5.0.12 的 openAIModelRejectsReasoningEffort。
package modeladapter

import "strings"

// reasoningEffortRejectingModelSubstrings 对应 ultra 解码的模型匹配表（grok-composer-2.5 等）。
var reasoningEffortRejectingModelSubstrings = []string{"composer"}

// openAIEffectiveReasoningEffort 返回模型实际可用的 reasoning_effort；模型拒绝时返回空（剥离该参数）。
func openAIEffectiveReasoningEffort(modelID, effort string) string {
	effort = strings.TrimSpace(effort)
	if effort == "" || openAIModelRejectsReasoningEffort(modelID) {
		return ""
	}
	return effort
}

// openAIModelRejectsReasoningEffort 判断模型是否拒绝 reasoning_effort 参数。
func openAIModelRejectsReasoningEffort(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	for _, sub := range reasoningEffortRejectingModelSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// stripOpenAIReasoningForUnsupportedModel 从请求体 map 剥离 reasoning 相关键（覆盖 override 路径）。
func stripOpenAIReasoningForUnsupportedModel(body map[string]any, modelID string) {
	if !openAIModelRejectsReasoningEffort(modelID) {
		return
	}
	delete(body, "reasoning_effort")
	delete(body, "reasoning")
}
