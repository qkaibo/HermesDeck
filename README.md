# HermesDeck

**Hermes Agent × PilotDeck 融合项目**

取 Hermes Agent 的 Agent 核心循环、记忆系统、技能引擎和网关平台，融合 PilotDeck 的轻量 Go 运行时、结构化工具链和浏览器自动化构建的下一代 AI 智能体系统。

## 架构全景

```
用户 → [Go Runtime (PilotDeck 前端)] ←→ gRPC → [Python Sidecar (Hermes 核心)]
              │                                          │
              ├─ 浏览器自动化 (MCP)                      ├─ Agent 循环
              ├─ 文件操作 / 命令执行                      ├─ 记忆系统 (SQLite FTS5)
              ├─ Web 搜索 / 抓取                          ├─ 技能引擎 (SKILL.md)
              ├─ CLI / TUI / Web 通道                     ├─ 网关 (Telegram/Discord/...)
              └─ 后台任务系统                              └─ Curator 自进化
```

## 快速开始

```bash
git clone https://github.com/your-org/hermesdeck.git
cd hermesdeck

# 一键启动（Docker Compose）
make up

# 开发模式
make dev
```

## 技术栈

| 层 | 技术 |
|---|------|
| **Go Runtime** | Go 1.22+, gRPC, chi router, tcell (TUI) |
| **Python Sidecar** | Python 3.11+, Hermes Agent 核心, gRPC |
| **通信协议** | gRPC (双向流) |
| **持久化** | SQLite (FTS5 全文搜索) |
| **部署** | Docker Compose / 单体开发模式 |
| **浏览器** | Playwright (通过 MCP) |

## 项目结构

```
hermesdeck/
├── proto/              # gRPC 协议定义
├── src/go/             # Go Runtime (PilotDeck 部分)
├── src/python/         # Python Sidecar (Hermes 部分)
├── docker/             # Docker 构建文件
├── docs/               # 文档
└── scripts/            # 工具脚本
```
