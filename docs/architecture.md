# HermesDeck 架构文档

## 1. 系统架构

```
┌──────────────────────────────────────────────────────────────────┐
│                        HermesDeck                                 │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐     │
│  │                   会话管理层 (Session Layer)               │     │
│  │  统一管理用户会话，路由到 Go Runtime 或 Python Sidecar     │     │
│  └──────────────────────┬──────────────────────────────────┘     │
│                          │                                        │
│          ┌───────────────┴───────────────┐                       │
│          ▼                               ▼                       │
│  ┌──────────────────┐        ┌──────────────────────┐          │
│  │   Go Runtime      │ gRPC  │   Python Sidecar      │          │
│  │   (PilotDeck)     │◄─────►│   (Hermes Core)       │          │
│  │                   │        │                       │          │
│  │  ┌─────────────┐  │        │  ┌─────────────────┐  │          │
│  │  │ CLI / Web   │  │        │  │ Agent Loop      │  │          │
│  │  │ TUI / Chat  │  │        │  │ (conversation)  │  │          │
│  │  ├─────────────┤  │        │  ├─────────────────┤  │          │
│  │  │ 工具引擎    │  │        │  │ MemoryManager   │  │          │
│  │  │ Browser MCP │  │        │  │ + FTS5 Search   │  │          │
│  │  │ File ops    │  │        │  ├─────────────────┤  │          │
│  │  │ Bash exec   │  │        │  │ Skills Engine   │  │          │
│  │  │ Web search  │  │        │  │ (SKILL.md)      │  │          │
│  │  ├─────────────┤  │        │  ├─────────────────┤  │          │
│  │  │ Plan Mode   │  │        │  │ Curator         │  │          │
│  │  │ Task System │  │        │  │ Gateway Plat.   │  │          │
│  │  └─────────────┘  │        │  └─────────────────┘  │          │
│  └──────────────────┘        └──────────────────────┘          │
└──────────────────────────────────────────────────────────────────┘
```

## 2. 通信协议

Go 和 Python 之间通过 **gRPC 双向流**通信。

### 2.1 核心消息流

```
User → Go Runtime CLI
         │
         ├── 工具命令 (file/bash) → 本地执行 → 返回
         │
         └── 需要 agent 推理 → gRPC ProcessMessage
                │
                ▼
         Python Sidecar → Hermes Agent Loop
                │
                ├── 文本回复 → gRPC stream → Go → User
                │
                └── 工具调用 → gRPC ExecuteTool → Go 执行 → 返回
                                │
                                ▼
                         Hermes 继续推理
                                │
                                ▼
                         最终回复 → User
```

### 2.2 协议定义

详见 `proto/hermesdeck.proto`，核心 RPC：

| RPC | 方向 | 说明 |
|-----|------|------|
| `ProcessMessage` | Go → Python | 用户消息 → Agent，流式返回 |
| `ExecuteTool` | Python → Go | 工具调用请求 → 执行结果 |
| `RegisterTools` | Go → Python | 注册 Go 端工具列表 |
| `MemoryOp` | Go → Python | 记忆存取/搜索 |
| `SkillOp` | Go → Python | 技能管理 |
| `HealthCheck` | 双向 | 心跳检测 |

### 2.3 工具注册机制

1. Go Runtime 启动时调用 `RegisterTools` 向 Python 注册工具
2. 每个工具包含：`name`, `description`, `parameters_json_schema`
3. Python 端 `ToolAdapter` 维护工具列表
4. Hermes Agent 通过 `tool_use` 选择工具
5. 工具调用通过 `ExecuteTool` 回 Go 端执行
6. 执行结果返回 Python，Agent 继续推理

## 3. 会话模型

### 3.1 会话生命周期

```
创建会话 (Create)
  │
  ├── go_local 模式 (默认)
  │   纯工具操作，不经过 Hermes
  │   适用于：文件编辑、命令执行、浏览器操作
  │
  └── hermes_remote 模式
      消息经过 Hermes Agent
      适用于：复杂推理、需要记忆的任务
```

### 3.2 会话状态

| 状态 | 说明 |
|------|------|
| `active` | 会话活跃，可接收消息 |
| `thinking` | Agent 正在推理中 |
| `idle` | 等待用户输入 |
| `error` | 发生错误 |

## 4. 部署架构

### 4.1 Docker Compose（生产）

```
┌─────────────────────┐       ┌──────────────────────┐
│   go-runtime:50051  │       │  python-sidecar:50052 │
│    (gRPC 服务端)    │◄─────►│   (gRPC 服务端)       │
│                     │       │                       │
│   Web:8080          │       │  数据持久化: /data    │
└─────────────────────┘       └──────────────────────┘
```

### 4.2 单体模式（开发）

```
Go 进程启动 Python 子进程
Go: 127.0.0.1:50051  ←→  Python: 127.0.0.1:50052
```

## 5. 关键技术决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 通信协议 | gRPC | 双向流、强类型、高性能 |
| Go 工具引擎 | 注册中心模式 | Python 无需关心工具实现细节 |
| 记忆存储 | SQLite FTS5 | 零依赖、全文搜索、嵌入式中 |
| 技能格式 | SKILL.md | 与 Hermes 兼容的纯文本格式 |
| 会话路由 | Go 端决策 | 减少 gRPC 调用延迟 |
