# HANDOFF.md — 当前工作状态交接

> 最后更新：2026-08-11（branch `fix/perf-leaks`，v0.0.46）

## 已完成的工作

### Fix 11：provider 服务端重试 — 解决"断流"（本次核心修复）

- **问题**：使用自定义 provider 时，上游瞬时错误（429 限流、502/503/504、容量不足、连接重置/EOF）直接断流，agent 会话被取消/完成，无可读错误
- **根因**：`agent/model/retry.go` 的 `doProviderRequestWithRetry` 是一次性 `client.Do` 裸壳；forwarder 层 `isRetryableStreamError` 只匹配网络/连接层错误，429/5xx/容量错误一律不重试 → failStream → 断流。（对照 CursorUltra 5.0.12 逆向确认差距。）
- **修复文件**：`internal/backend/agent/model/retry.go`、`retry_classify.go`（新）、`openai.go`、`openai_reasoning.go`（新）、`http_error.go`
- **要点**：
  - `doProviderRequestWithRetry` 改为真实重试循环（传输 + 429/502/503/504 状态瞬时即重试；容量长退避；不可重试立即返回；最大 3 次；零内容投递前重试，天然安全）
  - 分类子串表从 ultra 二进制解码；reasoning 自适应剥离 composer 系模型 `reasoning_effort`；重试耗尽返回可读提示
  - 三个调用点（openai.go:524/1003、anthropic.go:320）零改动；forwarder 零事件重试保留为第二层兜底
- **测试**：`retry_classify_test.go` / `retry_test.go`（含 adapter 端到端 429→正常流式）/ `openai_reasoning_test.go`，model + forwarder 全量回归通过
- **版本**：`0.0.45` → `0.0.46`
- **状态**：已推送到 `fix/perf-leaks`，触发 GitHub Actions 构建

### Fix 1：Anthropic 批量导入 baseURL + 模型名可编辑（`33079da`）

- **问题**：批量导入 Anthropic 模型时 baseURL 硬编码，无法用第三方端点；模型名不可改
- **修复文件**：`frontend/src/views/ModelConfig.vue`
- **状态**：已合并，推送到 `fix/perf-leaks`

### Fix 2：ModelConfig 按供应商分组（`33079da`）

- **问题**：模型列表无分组，供应商多时滚动困难
- **修复文件**：`frontend/src/views/ModelConfig.vue`
- **状态**：已合并，推送到 `fix/perf-leaks`

### Fix 3：TLS 握手失败导致 agent 会话中断（`4bf92f7`）——本次核心修复

- **问题**：使用自定义模型时，agent 会话频繁 `outcome=cancelled`，跨多个模型普遍出现
- **根因诊断过程**：
  1. 用户提供会话 ID `20793613-4120-4150-964c-d1bb159cc7fa`
  2. 分析 Cursor 结构化日志，发现 `Connect error in unary AI connect` → 重试 3 次 → `CancelledError`
  3. 分析 `~/.cursor-local-assistant-v2/logs/app.log`，发现每 30-60 秒重复出现：
     ```
     goproxy: WARN: Cannot handshake client api3.cursor.sh:443 remote error: tls: unknown certificate
     goproxy: WARN: Cannot handshake client metrics.cursor.sh:443 remote error: tls: unknown certificate
     ```
  4. **根因**：`api3.cursor.sh` 使用 mTLS，代理 MITM 拦截时无 Cursor 客户端证书 → 服务端拒绝 → auth/session gRPC 失败 → Cursor 判断 backend 不可用 → 取消 agent turn
- **修复**：`internal/mitm/service.go` 新增 `isTunnelOnlyHost()`，CONNECT handler 对 `api3.cursor.sh` 和 `metrics.cursor.sh` 返回 `OkConnect`（透明 TCP tunnel，不做 MITM）
- **影响范围**：`api2.cursor.sh` 的 AI 推理拦截完全不受影响
- **测试**：`go test ./internal/mitm/...` 通过
- **状态**：已推送到 `fix/perf-leaks`，等待 GitHub Actions 构建

### Fix 4：Provider pass 无上限导致 agent 会话无限循环（`f4a0584`）

- **问题**：同一个 request_id 在6分钟内执行了42次 provider pass（pass 1-42），Cursor 客户端侧超时后强制中断会话
- **根因**：`forwarder/actor.go` 的 `handleProviderDoneEvent` 在 `hadToolInvocation=true` 时无条件继续循环，没有上限守卫
- **触发场景**：用户要求 "多 agents 同步推进" → 模型连续调用 `Task` 工具派生子 agents → 子任务每次完成后继续触发新 pass → 无限循环
- **修复**：
  - `internal/backend/forwarder/service.go`：新增常量 `maxProviderPassesPerTurn = 50`
  - `internal/backend/forwarder/actor.go`：在循环继续条件前检查 pass 数，超限则 `failStream("max_provider_passes_exceeded")`
- **状态**：已推送到 `fix/perf-leaks`

### Fix 5：`finish_reason=length` 静默中止（`fc567f7`）

- **问题**：使用 arkcoding deepseek-v4（`reasoningEffort: max`）时，agent 在 ~21 pass 后无任何错误日志，直接停止。最后响应"现在运行测试"，但 tool call 未执行
- **根因**：deepseek-v4 每 pass 消耗大量 reasoning tokens（max effort），经 21 pass 后输出 token 预算耗尽 → `finish_reason=length` → tool call 被截断。后端把 `length` 当作正常 `stop` 处理，静默完成 turn，日志无任何错误，调用方看不到任何提示
- **修复**：`internal/backend/forwarder/actor.go` `handleProviderDoneEvent`：检测 `finish_reason=length && !hadToolInvocation` 时显式 `failStream("max_tokens_exceeded")` 并打印日志，用户在 Cursor 中看到明确错误
- **版本**：`0.0.40` → `0.0.41`
- **状态**：已推送到 `fix/perf-leaks`

### Fix 10：自动发布 + 自动更新（`484f563`，v0.0.45）

- **背景**：`internal/updater/manager.go` 已有完整更新管理器但从未接入 Release
- **修复**：
  - GitHub Actions 推送到 `main` 时自动编译 + 发布 GitHub Release
  - 生成 `update.json` manifest
  - 上传 `.tar.gz` 归档作为 Release asset
  - 旧版 Cursor 助手 20 分钟内自动检测并提示安装
- **状态**：已推送到 `fix/perf-leaks`

---

### Fix 9：stream idle watchdog — 快速检测上游挂起（`dbf5fab`，v0.0.44）

- **问题**：LongCat-2.0、deepseek-v4-pro 等重推理模型在 streaming 中途停止输出但不关闭 TCP 连接，proxy 等到 4 分钟 idle watchdog 才触发，远长于 Cursor 客户端超时
- **修复**：`service.go` `runProviderStream` 增加 60 秒 idle watchdog
  - 每收到 event 重置 timer
  - 60 秒无 event → 取消 context
  - 零事件 → 透明重试；部分事件 → failStream
- **状态**：已推送到 `fix/perf-leaks`

---

### Fix 8：provider stream 透明自动重试（`1bccbcc`，v0.0.43）

- **问题**：使用 LongCat-2.0（xhigh reasoning）、deepseek-v4-pro（high reasoning）时，agent 在 pass 中途停止，app.log 无错误，session 无声消失
- **根因**：上游 LLM 提供商在 streaming 空闲或单请求时长达到上限时静默关闭 TCP 连接。proxy 层将错误静默吞掉
- **修复**：`internal/backend/forwarder/service.go` `runProviderStream` 增加透明重试机制：
  - atomic counter 跟踪已投递给 actor 的事件数
  - 仅当零事件已投递且错误为可重试类型（连接重置、超时、EOF、TLS 握手失败）时自动重试
  - 指数退避 2s → 4s → 8s，最多 3 次
  - 已投递部分事件的失败不重试（避免重复执行工具），走 failStream 路径
- **状态**：已推送到 `fix/perf-leaks`

---

### Fix 6+7：stream 错误可见性 + 响应头超时（`74a0ca2`）

- **问题**：使用 LongCat-2.0（reasoningEffort=xhigh）、deepseek-v4-pro（reasoningEffort=high）时，agent 在 reasoning 中途停止，app.log 无任何错误
- **根因**：重推理模型在"深度思考"阶段不输出 token，上游 LLM 提供商（atmai.site / ark.cn / api.longcat.chat）在 streaming 空闲超过 N 秒后静默关闭 TCP 连接。`failStream` 只写 history metadata 不写 app.log，所以中断完全隐形
- **修复**：
  - Fix 6：`service.go` `failStream` 增加 `log.Printf`，所有 stream 失败现在出现在 app.log
  - Fix 7：`netproxy.go` `cloneDefaultTransport` 加 `ResponseHeaderTimeout=60s`，上游 drop 连接时60秒内就能检测到并报错，不再等 OS TCP keepalive（15+ 分钟）
- **版本**：`0.0.41` → `0.0.42`
- **状态**：已推送到 `fix/perf-leaks`

---

### 附：CoT 泄漏分析（未修复，外部依赖问题）

- **现象**：Cursor 聊天里出现 `"Wait, the user said "多 agents 同步推进", so let me use Task to delegate coherent work.}"` 这类推理文字
- **根因**：上游 OpenAI-compatible 代理（`atmai.site`/`tsyjzzz.com`）在 streaming response 里把推理内容直接混入 content text chunk，没有 `<think>` 标签包裹。`openAIThinkTagParser` 只过滤有 `<think>...</think>` 标签的推理块，无标签的内容直接作为 `TextDelta` 输出
- **代码路径**：`internal/backend/agent/model/openai.go` → `openAIThinkTagParser.Consume()` → 无标签推理内容 → `emitTextDelta` → Cursor 聊天
- **无法在代码层简单修复**：不加标签的推理内容与正常回复文本无法可靠区分，需要上游代理修复输出格式
- **缓解方案**（可选）：对接使用 `reasoning_content` 字段的代理（已在 `openai.go:562` 支持），或换用支持 Anthropic 原生 thinking block 的端点

```
branch: fix/perf-leaks
commits ahead of main: 多个（包含性能修复批次 + 上述三个 fix）
最新 commit: 4bf92f7
```

## 待办 / 未决问题

1. **`fix/perf-leaks` → `main` 的 PR**：所有修复都在这个分支，等 GitHub Actions 构建通过后可以发 PR 合并。
2. **`api2.cursor.sh` 上的非 AI 路径**：日志里偶尔出现 `api2.cursor.sh:443 remote error: tls: unknown certificate`，这些是 api2 上的非推理路径（如 feature flag 拉取）。目前 AI 推理正常工作，这些错误不影响功能，但如果以后 api2 的非 AI 路径也出问题，可以考虑对特定 path 做 bypass（需要先 MITM 再按路径判断，或改成 api2 仅对已知 AI 路径做 MITM）。
3. **前端测试**：Fix 1/2 的前端改动没有自动化测试覆盖，建议人工验证批量导入和分组展示功能。

## 关键诊断命令

```bash
# 看代理实时日志（TLS 错误）
tail -f ~/.cursor-local-assistant-v2/logs/app.log | grep -i "handshake\|tls\|error"

# 看 Cursor 结构化日志（会话中断）
ls ~/Library/Logs/Cursor/
grep -r "Connect error\|outcome=cancelled\|CancelledError" ~/Library/Logs/Cursor/

# 验证代理配置
cat ~/.cursor-local-assistant-v2/config.yaml

# 验证 Cursor HTTP proxy 设置
grep -E "proxy|disableHttp2" ~/Library/Application\ Support/Cursor/User/settings.json
```

## 重要约束（接手时必读）

- **不要**把 `api3.cursor.sh` 或 `metrics.cursor.sh` 改回 MITM 模式——这会恢复 TLS 握手失败
- **不要**修改 `/Applications/Cursor.app` 的客户端 bundle
- `internal/mitm/service.go` 修改后必须跑 `go test ./internal/mitm/...`
- 渠道 ID 是 baseURL+modelID+apiKey+displayName+openAIEndpoint 的 SHA-256 前 16 位，不是 modelID 本身
