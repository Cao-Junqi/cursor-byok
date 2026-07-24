# cursor-byok

> 把你自己的模型 API 接入 Cursor IDE，摆脱单一平台绑定。

本项目 fork 自 [leookun/cursor-byok](https://github.com/leookun/cursor-byok)，在原版基础上修复了多项性能问题并新增实用功能，由 [@Cao-Junqi](https://github.com/Cao-Junqi) 维护。

---

## 功能特性

- **BYOK（Bring Your Own Key）**：将 OpenAI 兼容接口或 Anthropic 接口直接接入 Cursor，不限供应商
- **批量导入模型**：输入 base URL 和 API Key，一键拉取供应商所有可用模型，勾选后批量添加，支持供应商名称前缀防止冲突
- **模型复制**：在已有模型卡片上点击「复制」，快速克隆配置只改模型 ID
- **统计自动刷新**：首页 Token 消耗统计每 30 秒自动刷新，无需手动点击
- **macOS HiDPI 支持**：修复 Retina 屏下界面模糊问题

---

## 相比原版的改动

### 性能修复

| 问题 | 根因 | 修复 |
|------|------|------|
| 内存占用 6.5 GB | `Backlog` 事件队列无限增长，从不 trim | 按订阅者游标实时裁剪 |
| 磁盘 I/O 200+ GB | 每次写 JSON 后调 `syncDirectory`（fsync）| 移除热路径 fsync |
| CPU 持续 180% | HTTP 超时 30000 秒导致 goroutine 堆积；`watchSyncEffect` 每 tick 同步序列化 localStorage | 超时改为 10 分钟；改为异步 `watchEffect` |
| 历史文件占满磁盘 | session 文件永久保留，无清理机制 | 启动时自动删除 30 天前的 session 目录 |

### 新功能

- **批量导入**：ModelConfig 页新增「批量导入」按钮，支持 OpenAI 兼容和 Anthropic 两种协议
- **统计自动刷新**：Token 消耗每 30 秒静默刷新
- **macOS HiDPI**：`NSHighResolutionCapable` 从错误的字符串 `"true"` 改为正确的布尔 `<true/>`

### 清理

- 移除内嵌广告系统（前端组件、后端路由、定时拉取全部删除）
- 日志 trim 阈值从 10k 提升到 50k 行，减少全文读写频率

---

## 安装

在 [Releases](https://github.com/Cao-Junqi/cursor-byok/releases) 下载最新 macOS 包，解压后拖入 Applications，首次启动需要执行：

```bash
xattr -cr /Applications/Cursor助手.app
```

双击启动后，App 以托盘形式运行，图标出现在**屏幕右上角菜单栏**，点击图标打开主界面。

---

## 快速上手

1. 点击主界面右上角「模型配置」
2. 点「批量导入」，填入 base URL 和 API Key
3. 获取模型列表后勾选需要的模型，填写供应商名称（可选前缀）
4. 点「添加选中」完成
5. 回主界面点「启动服务」，Cursor 即可通过本地代理使用你自己的模型

---

## 开发构建

```bash
# 依赖：Go 1.21+、Node.js 22+、yarn、protoc、wails3
task common:generate:proto
task build:darwin:arm64
```

---

## License

MIT — 基于 [leookun/cursor-byok](https://github.com/leookun/cursor-byok)
