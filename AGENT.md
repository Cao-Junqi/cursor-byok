# AGENT.md — cursor-byok 仓库 Agent 指引

## 这个仓库是什么

cursor-byok 是 **Cursor 助手**（`/Applications/Cursor助手.app`，bundle ID `com.cursor.wuxianxubei`）的源码仓库，由 [@Cao-Junqi](https://github.com/Cao-Junqi) 维护，fork 自 [leookun/cursor-byok](https://github.com/leookun/cursor-byok)。

核心功能：在本地运行一个 HTTP MITM 代理（默认 `127.0.0.1:18080`），拦截 Cursor IDE 对 `api2.cursor.sh` 的 AI 推理请求，转发给用户自定义的模型 API（OpenAI 兼容或 Anthropic 协议）。

## 技术栈

- **Go 后端**：Wails3 app，`internal/` 目录下的所有 Go 代码
- **Vue3 前端**：`frontend/` 目录，HeroUI 组件库
- **代理层**：`github.com/elazarl/goproxy`，入口 `internal/mitm/service.go`
- **backend/forwarder**：本地 AI 推理转发内核，`internal/backend/`
- **自动更新**：`internal/updater/manager.go`（检查、下载、验证、安装、重启），`internal/buildinfo/buildinfo.go`（版本、UpdateBaseURL）
- **配置**：`~/.cursor-local-assistant-v2/config.yaml`
- **数据目录**：`~/.cursor-local-assistant-v2/`
- **配置**：`~/.cursor-local-assistant-v2/config.yaml`
- **数据目录**：`~/.cursor-local-assistant-v2/`

## 关键文件地图

```
internal/
  mitm/service.go          # goproxy MITM 代理，CONNECT handler，域名 whitelist
  backend/
    host.go                # 后端组装入口
    server/route.go        # HTTP 路由，routing.mode 判断
    server/policy.go       # PolicyMiddleware，本地/上游分支
    forwarder/service.go   # BidiAppend/RunSSE 主链路
    forwarder/file_store.go# history state.json + context.json 读写
    agent/model/
      router.go            # 模型渠道选择
      openai.go            # OpenAI 兼容适配
      anthropic.go         # Anthropic 协议适配
  client/
    config.go              # 用户配置结构体
    lifecycle.go           # 服务启动/停止
  appdata/paths.go         # 固定路径：~/.cursor-local-assistant-v2/
frontend/src/
  views/ModelConfig.vue    # 模型管理页（批量导入、分组展示）
  state/                   # Pinia store
```

## 代理拦截逻辑（重要）

Cursor 的所有 HTTPS 请求经过 `http.proxy: http://127.0.0.1:18080`（由 app 自动写入 Cursor settings.json）。

`isWhitelistedRelayHost()` 控制哪些域名被 MITM 拦截：
- `api2.cursor.sh`：AI 推理路径，**MITM**，转发给自定义模型
- `api3.cursor.sh`：auth/session/unary gRPC，**TCP tunnel（不做 MITM）**，直通原始 Cursor 服务器
- `metrics.cursor.sh`：埋点，**TCP tunnel（不做 MITM）**
- 其他 `*.cursor.sh`：MITM，转发给 backend

`isTunnelOnlyHost()` 列出必须 TCP tunnel 的主机（mTLS 域名，代理没有 Cursor 客户端证书）。

**不要把 api3.cursor.sh 或 metrics.cursor.sh 改回 MITM 模式**，否则会恢复 TLS 握手失败和 agent 会话中断问题。

## routing.mode 语义

- `local`：AI 推理拦截后转发给 `backendListenAddr`（默认 `127.0.0.1:18090`），由 forwarder 路由到自定义模型
- `upstream`：pass-through，直连真实 Cursor 服务器

## 持久化布局

```
~/.cursor-local-assistant-v2/
  config.yaml              # 用户配置（modelAdapters, routing.mode 等）
  data/ca.crt              # MITM CA 证书（注入 Cursor 信任链）
  history/
    usage.json             # 全局 token 消耗聚合
    <conversationId>/
      state.json           # 会话元数据和当前 loop 状态
      context.json         # append-only 语义历史（prompt replay 源）
      conversation.lock
  logs/app.log             # 代理运行日志
```

## 模型渠道 ID

渠道唯一 ID = `baseURL + modelID + apiKey + displayName + openAIEndpoint` 的 SHA-256 前 16 个十六进制字符。排查模型选择问题时优先用渠道 ID，不要只看 modelID。

## 构建

```bash
task common:generate:proto   # 生成 protobuf
task build:darwin:arm64      # 构建 macOS arm64
```

GitHub Actions 自动构建触发条件：push 到任意分支（见 `.github/workflows/`）。

## 常见排查入口

| 现象 | 先看 |
|------|------|
| agent 会话中断 / outcome=cancelled | `logs/app.log` 中的 TLS/handshake 错误；Cursor 结构化日志中的 `Connect error in unary AI connect` |
| 自定义模型无响应 | `state.json.last_provider_call`；`logs/app.log` 中的转发错误 |
| 模型配置丢失 | `config.yaml` 中的 `modelAdapters` 列表 |
| 代理未生效 | Cursor settings.json 中的 `http.proxy` 和 `cursor.general.disableHttp2` |
| MITM CA 不信任 | `data/ca.crt` 是否正确注入；`logs/app.log` 中的 CA info |
| provider stream 中途断开 | `logs/app.log` 中的 `stream failed`、`stream retry`、`stream idle timeout` |
| 自动更新失败 | `logs/app.log` 中的 `检查更新失败`、`download update` 错误 |

## 最新修复记录（v0.0.57）

| Fix | 文件 | 说明 |
|-----|------|------|
| 5 | `internal/backend/forwarder/actor.go` | v0.0.41 初步处理：`finish_reason=length` 时显式 failStream |
| 6 | `internal/backend/forwarder/service.go` | failStream 加 log.Printf |
| 7 | `internal/netproxy/netproxy.go` | cloneDefaultTransport 加 ResponseHeaderTimeout=60s |
| 8 | `internal/backend/forwarder/service.go` | runProviderStream 零事件失败透明重试（3次退避） |
| 9 | `internal/backend/forwarder/service.go` | runProviderStream 60s idle watchdog |
| 10 | `.github/workflows/build-macos.yml` | 推送到 main 自动编译 + 发布 Release（macOS+Windows） |
| 11 | `internal/backend/forwarder/actor.go`、`reminders.go` | v0.0.54：保留截断输出，写入去重恢复上下文并直接续跑 provider pass |
| 12 | `.github/workflows/build-macos.yml` | 修正资产下载路径，并将 Release tag 显式绑定实际构建 SHA |
| 13 | `internal/backend/forwarder/actor.go` | v0.0.55：统一识别 `length`、`max_output_tokens`、`max_tokens` 等 provider Token 截断原因 |
| 14 | `internal/backend/forwarder/actor.go`、`reminders.go` | v0.0.56：reasoning-only 正常完成时注入一次恢复上下文并续跑，重复空完成显式失败 |
| 15 | `internal/backend/forwarder/actor.go`、`reminders.go` | v0.0.57：纯续做指令遇到“有 reasoning、有进度文本、零工具”的正常完成时，注入一次执行恢复上下文并续跑 |

### v0.0.57 待实机确认

v0.0.56 已确认安装包、Release tag 和运行中二进制一致；最新实机请求证明剩余问题是“有可见进度文本但零工具”的正常完成，不是 Token 截断或旧包。v0.0.57 本地测试已覆盖续做识别、模式边界、恢复上下文去重、重复无动作失败和工具完成后的正常总结，仍需用安装后的 Release 产物复现一次纯“继续”。

验收标准：

- plist 显示 `0.0.57`
- 安装目录内真实二进制包含 `C0.0.57`、`continuation_execution_recovery`、`continuation_without_action`、`recovery=resume`
- 纯“继续”先返回进度文本但零工具时，`app.log` 出现 `continuation without action ... recovery=resume`，随后同一 turn 继续 provider pass 并执行工具

```bash
defaults read /Applications/Cursor助手.app/Contents/Info CFBundleShortVersionString
strings /Applications/Cursor助手.app/Contents/MacOS/Cursor助手 | rg 'C0\.0\.57|continuation_execution_recovery|continuation_without_action|recovery=resume'
```

## 约束

- 不要修改已安装的 `/Applications/Cursor.app` 客户端 bundle
- 不要把 api3.cursor.sh / metrics.cursor.sh 改回 MITM
- 不要在 `isTunnelOnlyHost` 里删除已有的 mTLS 域名
- 涉及 `internal/mitm/service.go` 的修改必须保证 `go test ./internal/mitm/...` 通过
