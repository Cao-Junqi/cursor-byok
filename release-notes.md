# Release Notes

## [Unreleased] — v0.0.53

### fix: 输出 Token 截断后可靠自动续跑

**症状**：使用 deepseek-v4 等高 reasoningEffort 模型，在输出大量内容或多次调用工具后，模型突然停止，Loading 图标变为发送箭头，且应用内没有任何错误提示。
**根因**：`v0.0.52` 用不存在的 `run_terminal_cmd` 工具模拟续跑，但后续状态判断仍读取注入前的 `hadToolInvocation`。无真实工具完成时，`length` 最终仍被当成成功结束，自动续跑没有发生。
**修复**：`finish_reason=length` 时保存已生成内容，并通过内部恢复上下文直接启动下一次 provider pass；不再伪造客户端工具调用。已完成的真实工具仍按原流程等待结果，终止型工具不会被强制续跑，50 pass 上限继续防止无限循环。
**验证**：新增续跑决策与恢复上下文去重测试；完整 Go 测试、forwarder race test 和前端生产构建均通过。

### fix: 自动更新清单使用正确的 Release 下载地址

**问题**：`update.json` 使用不存在的 GitHub `/downloads/` 路径，可能出现检测到新版本但下载不到对应安装包。
**修复**：平台资产 URL 改为 GitHub Release 的 `/releases/download/v<version>/...` 格式。

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
