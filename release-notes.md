# Release Notes

## [Unreleased] — v0.0.60

### fix: provider 服务端重试 — 解决"断流"（Fix 18）

**症状**：使用自定义 provider 时，上游瞬时错误（429 限流、502/503/504、容量不足、连接重置/EOF）直接断流，agent 会话被取消/完成，无可读错误。

**根因**：`agent/model/retry.go` 的 `doProviderRequestWithRetry` 是一次性 `client.Do` 裸壳；forwarder 层 `isRetryableStreamError` 只匹配网络/连接层错误，429/5xx/容量错误一律不重试 → failStream → 断流。（对照 CursorUltra 5.0.12 逆向确认差距。）

**修复**：把 ultra 的 provider 弹性层移植进 `agent/model`：
- `retry.go`：`doProviderRequestWithRetry` 改为真实重试循环——传输错误 + 429/502/503/504 状态错误瞬时即重试；容量错误长退避；不可重试（401/403/积分不足等）立即返回；最大 3 次；在零内容投递前重试，天然安全
- `retry_classify.go`（新）：`isTransientProviderStreamError` / `isTransientProviderDialError` / `isNonRetryableProductError` / `isCapacityStyleProviderMessage` + `sleepProviderRetry`(200ms×2ⁿ≤800ms) / `sleepProviderCapacityRetry`(800ms/1.5s/2s)，子串表从 ultra 二进制解码
- `openai.go` + `openai_reasoning.go`（新）：reasoning 自适应——composer 系模型剥离 `reasoning_effort`，防 400/token 异常
- `http_error.go`：`mapTechnicalProviderMessage` / `extractProviderErrorMessage`，重试耗尽返回可读提示
- 三个调用点（openai.go:524/1003、anthropic.go:320）零改动；forwarder 零事件重试保留为第二层兜底
- 测试：`retry_classify_test.go` / `retry_test.go`（含 adapter 端到端 429→正常流式）/ `openai_reasoning_test.go`，model + forwarder 全量回归通过

**版本**：`0.0.59` → `0.0.60`

---

### fix: 无 Todo 续做不再被历史工具结果误判为完成

**问题**：纯“继续”请求第一次恢复后，如果历史上已经有过工具结果，但恢复窗口内没有执行新工具，仍可能被误判为合法总结并提前结束。
**修复**：完成判定同时检查恢复计数；只有真实工具调用将计数重置后，才允许恢复后的无工具总结。否则记录 `continuation_without_action` 并明确失败，避免静默中断。

### fix: provider 流空闲超时统一使用配置值

**问题**：forwarder 另有一套写死的 120 秒首事件/60 秒事件间隔 watchdog，早于配置中的 `providerStreamIdleTimeout: 240` 取消重推理请求，导致约 2 分钟自动中断。
**修复**：forwarder 与 provider adapter 统一使用 `providerStreamIdleTimeout`，首次事件和后续事件间隔均按该值计时，超时日志记录实际时长。
**验证**：新增无 Todo 恢复判定、forwarder 超时配置解析和配置管理器读取测试；完整 Go 测试、forwarder race test 与前端生产构建均已通过。

### fix: 未完成 Todo 时禁止提前结束 Agent 轮次

**症状**：v0.0.57 已安装且 `continuation_execution_recovery` 确实触发，但同一请求在后续 pass 仍可能只输出“现在更新登录页”便结束。
**实机证据**：请求 `d5265918-987e-4875-a977-446fcb98d194` 在 v0.0.57 下运行到 provider pass 4；Cursor 保存的结构化 Todo 中第 6 项“增加 Passkey (WebAuthn) 认证”仍为 `in_progress`，最后一个 bubble 没有工具调用。
**根因**：v0.0.57 只用“本轮之前是否有过工具结果”判断是否可以总结；只要前面成功写过一个文件，后续“下一步还没执行”的文本也会被误判为最终答复。
**修复**：Agent 纯续做请求的完成判定接入结构化 Todo。只要仍有 `pending` 或 `in_progress` 项，即使之前有工具结果，当前无工具的正常完成也必须续跑；每次真实工具调用会重置无动作恢复计数，连续再次无工具则明确失败，避免无限循环。Todo 全部 `completed/cancelled` 后才允许总结。
**验证**：覆盖未完成 Todo、有工具结果后的续跑、重复无动作失败、工具调用后的计数重置以及终态 Todo 的正常完成；完整 Go 测试、forwarder race test 与前端生产构建通过。

## v0.0.57

### fix: “继续”不再只描述下一步便结束

**症状**：安装 v0.0.56 后，Agent 收到“继续”仍可能在数秒内结束，留下类似“简化成只走一次验证”的方案描述，却没有执行任何工具。
**实机证据**：请求 `0ec77905-4798-476d-ac3b-25dcafa0e404` 在约 7 秒内以正常完成结束，`input_tokens=88175`、`output_tokens=355`；该 pass 同时产生 reasoning 和可见文本，但工具调用数为 0。因此这次不是 Token 截断、超时或 reasoning-only 空完成。
**根因**：v0.0.56 只恢复“有 reasoning、无文本、无工具”的完成；一旦模型输出了进度或方案文本，后端便把它当作正常最终答复。
**修复**：在 Agent 模式识别纯“继续/接着做/continue”等续做指令，明确要求执行未完成工作；若正常完成时仍只有 reasoning 和可见文本而没有工具调用，写入一次 `continuation_execution_recovery` 上下文并续跑。恢复后仍拒绝执行则以 `continuation_without_action` 明确失败，避免无限循环或伪装完成；Ask 模式、具体追问和已经产生工具结果的最终总结不受影响。
**验证**：覆盖续做指令边界、Agent/Ask 模式、首次恢复、重复无动作失败、已有工具调用和工具结果后的正常完成；完整 Go 测试、forwarder race test 与前端生产构建通过。

## v0.0.56

### fix: 仅产生内部推理时继续当前任务

**症状**：回复显示“继续”后，不到 2 分钟便自动结束；最后只留下类似“现在运行 tsc”的内部推理，计划中的工具没有执行，也没有用户可见答复。
**根因**：OpenAI Responses 可能以 `finish_reason=completed` 结束一个只包含 reasoning 的响应。后端此前把它视为正常完成，即使该 pass 没有文本也没有工具调用。
**修复**：识别 `completed`、`stop`、`message_stop`、`end_turn` 下的 reasoning-only 空完成，写入一次 `empty_completion_recovery` 上下文并继续下一次 provider pass；如果恢复后再次空完成则明确失败，50 pass 上限继续防止无限循环。
**验证**：覆盖 Responses/Chat/Anthropic 正常结束原因、正常文本、已有工具调用、恢复上下文去重和重复空完成；完整 Go 测试、forwarder race test 与前端生产构建通过。

## v0.0.55

### fix: 兼容 OpenAI Responses 的 Token 截断原因

**问题**：OpenAI Chat 使用 `finish_reason=length`，但 OpenAI Responses 使用 `response.incomplete` / `max_output_tokens`，Anthropic 使用 `max_tokens`。旧判断只识别 `length`，因此 Responses 截断会被误认为正常完成。
**修复**：统一识别各 provider 的 token-limit 原因，保留已生成内容并通过恢复上下文续跑；日志保留 provider 原始 `finish_reason`。

### fix: 输出 Token 截断后可靠自动续跑

**症状**：使用 deepseek-v4 等高 reasoningEffort 模型，在输出大量内容或多次调用工具后，模型突然停止，Loading 图标变为发送箭头，且应用内没有任何错误提示。
**根因**：`v0.0.52` 用不存在的 `run_terminal_cmd` 工具模拟续跑，但后续状态判断仍读取注入前的 `hadToolInvocation`。无真实工具完成时，`length` 最终仍被当成成功结束，自动续跑没有发生。
**修复**：`finish_reason=length` 时保存已生成内容，并通过内部恢复上下文直接启动下一次 provider pass；不再伪造客户端工具调用。已完成的真实工具仍按原流程等待结果，终止型工具不会被强制续跑，50 pass 上限继续防止无限循环。
**验证**：新增续跑决策与恢复上下文去重测试；完整 Go 测试、forwarder race test 和前端生产构建均通过。

### fix: 自动更新清单使用正确的 Release 下载地址

**问题**：`update.json` 使用不存在的 GitHub `/downloads/` 路径，可能出现检测到新版本但下载不到对应安装包。
**修复**：平台资产 URL 改为 GitHub Release 的 `/releases/download/v<version>/...` 格式。

### fix: Release tag 绑定实际构建提交

**问题**：仓库默认分支仍指向旧分支，GitHub Action 未指定 `target_commitish` 时会让新 tag 指向旧提交，即使上传的二进制来自最新 `main`。
**修复**：发布步骤显式使用 `${{ github.sha }}` 创建 tag，使源码 tag、构建提交和发布产物保持一致。

### fix: 修复外接非 Retina 显示器时界面滑动模糊的问题

**症状**：客户端程序界面模糊，在不同分辨率下显示器滑动不清晰。
**根因**：Wails 3 客户端的 `Backdrop` 配置使用了 `MacBackdropLiquidGlass` 毛玻璃半透明特效，与不透明的深色背景色混合叠加，导致在外部非高清显示器渲染时性能下降，产生抗锯齿异常和滑动撕裂。
**修复**：修改 `internal/app/runner.go` 主窗口属性，将 `Backdrop` 改为 `application.MacBackdropNone` 取消模糊层特效，恢复系统原生渲染模式，彻底解决滑动及文本发虚问题。

---

### feat: 自动发布 + 自动更新（Fix 10）

**背景**：`internal/updater/manager.go` 已有完整的更新管理器（检查、下载、验证、安装、重启），但从未接入实际 Release。

**修复**：
- `.github/workflows/build-macos.yml` 推送到 `main` 时自动编译 + 发布 GitHub Release
- 生成 `update.json` manifest（版本、checksum、平台下载 URL）
- 上传 `.tar.gz` 归档（updater 格式）作为 Release asset
- 旧版 Cursor 助手 20 分钟内自动检测到新 Release，提示用户一键安装

---

### fix: stream idle watchdog — 快速检测上游挂起（Fix 9）

**症状**：使用 LongCat-2.0、deepseek-v4-pro 等重推理模型时，agent 在 pass 中途停止，app.log 无错误，session 无声消失。

**根因**：上游 LLM 提供商在 streaming 中途停止输出但不关闭 TCP 连接（模型"深度思考"阶段）。proxy 会一直等到模型层的 4 分钟 idle watchdog 才触发——远长于 Cursor 客户端超时（10-20 秒），所以用户看到 session 突然中断。

**修复**：`service.go` `runProviderStream` 增加 60 秒 idle watchdog：
- 每收到一个 event 重置 timer
- 60 秒无新 event → 取消 context
- 零事件取消 → 触发透明重试
- 部分事件取消 → 触发 failStream，agent 可见错误

---

### fix: provider stream 自动重试（Fix 8）

**症状**：使用 LongCat-2.0（reasoningEffort=xhigh）、deepseek-v4-pro（reasoningEffort=high）等重推理模型时，agent 在 pass 中途停止，app.log 无错误，session 无声消失。

**根因**：上游 LLM 提供商（atmai.site / ark.cn / api.longcat.chat）在 streaming 连接空闲（重推理思考期间）或单请求时长达到上限时静默关闭 TCP 连接。proxy 层将错误静默吞掉，agent 看到的就是 session 突然中断。

**修复**：`service.go` `runProviderStream` 增加透明重试机制：
- 用 atomic counter 跟踪已投递给 actor 的事件数
- 仅当 **零事件已投递** 且错误为 **可重试类型**（连接重置、超时、EOF、TLS 握手失败等）时自动重试
- 指数退避：2s → 4s → 8s，最多 3 次重试
- 已投递部分事件的失败不重试（避免重复执行工具），走原有 failStream 路径

---

### fix: `finish_reason=length` 静默中止（Fix 5）

**症状**：使用 deepseek-v4 / 高 reasoningEffort 模型时，agent 在 20+ pass 后无任何 error 日志，直接停止。最后一条可见响应形如"现在运行测试"，但 tool call 未执行。

**根因**：模型输出 token 耗尽（`finish_reason=length`），tool call 被截断丢弃。后端将 `length` 视为正常 `stop`，静默完成 turn，调用方看不到任何错误。

**修复**：`actor.go` `handleProviderDoneEvent` — 检测 `finish_reason=length && !hadToolInvocation` 时，显式 `failStream("max_tokens_exceeded")` 并记录日志，用户在 Cursor 中将看到明确错误而非沉默停止。

---

### fix: TLS 握手失败导致 agent 会话中断 (`4bf92f7`)

**症状**：使用自定义模型时，agent 会话频繁出现 `outcome=cancelled`。日志：
```
goproxy: WARN: Cannot handshake client api3.cursor.sh:443 remote error: tls: unknown certificate
goproxy: WARN: Cannot handshake client metrics.cursor.sh:443 remote error: tls: unknown certificate
```

**根因**：`api3.cursor.sh` / `metrics.cursor.sh` 使用 mTLS，要求客户端证书。代理 MITM 拦截时没有 Cursor 的真实客户端证书 → 服务端拒绝 → Cursor auth/unary gRPC 调用（`GetEmail`、`ServerTime`）失败 → `Connect error in unary AI connect` → 重试 3 次 → `CancelledError` → 会话中断。

**修复**：`internal/mitm/service.go` 增加 `isTunnelOnlyHost()`，对 `api3.cursor.sh` 和 `metrics.cursor.sh` 返回 `OkConnect`（透明 TCP tunnel），不做 MITM。`api2.cursor.sh` 的 AI 推理拦截不受影响。

---

### fix: Anthropic 批量导入支持自定义 baseURL 及可编辑模型名 (`33079da`)

- 批量导入弹窗：Anthropic 协议新增 baseURL 输入框，不再硬编码官方端点
- 模型列表加载后每行可编辑 displayName，方便区分同一供应商多个 endpoint
- 修复批量导入时 baseURL 未传给 addModelAdapter 的 bug

### fix: ModelConfig 按供应商分组展示 (`33079da`)

- 模型管理页按 `供应商名/模型名` 前缀格式自动分组，每组可折叠
- 新增「全部展开/折叠」快捷按钮

---

## 历史版本

### 性能修复批次 (`e9b4f9b`, `cef9216`)

| 问题 | 修复 |
|------|------|
| 内存占用 6.5 GB | Backlog 队列按订阅者游标实时裁剪，不再无限增长 |
| 磁盘 I/O 200+ GB | 移除热路径 syncDirectory（fsync） |
| CPU 持续 180% | HTTP 超时从 30000s 改为 10min；watchSyncEffect 改异步 |
| 历史文件占满磁盘 | 启动时自动清理 30 天前 session 目录 |
| session 历史磁盘读写 | Go 侧改内存 TTL 缓存 |

### 功能新增

- 批量导入：支持 OpenAI 兼容和 Anthropic 两种协议，供应商名称前缀防冲突
- 统计自动刷新：首页 Token 消耗每 30 秒静默刷新
- macOS HiDPI：NSHighResolutionCapable 从错误字符串 "true" 改为布尔 true

### 清理

- 移除内嵌广告系统（前端组件、后端路由、定时拉取全部删除）
- 日志 trim 阈值从 10k 提升到 50k 行
