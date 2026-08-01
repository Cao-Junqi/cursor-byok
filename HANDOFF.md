# HANDOFF.md — 当前工作状态交接

> 最后更新：2026-08-02（commit `4bf92f7`，branch `fix/perf-leaks`）

## 已完成的工作

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

## 当前分支状态

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
