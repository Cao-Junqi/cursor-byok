# Release Notes

## [Unreleased] — fix/perf-leaks — v0.0.41

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
