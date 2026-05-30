# Plan: HermesDeck + PilotDeck 前端打通

## 目标

让 PilotDeck 前端（React UI，端口 18789）通过 HermesDeck Go 后端进行聊天交互，后端使用 Hermes Agent (Python) 处理 AI 推理。

## 架构设计

```
用户浏览器 → PilotDeck 前端 (18789)
                ↓ WebSocket (PilotDeck Gateway Protocol)
         HermesDeck Go Runtime (WebSocket Gateway, :18988)
                ↓ gRPC ProcessMessage (stream)
         HermesDeck Python Sidecar (Hermes Agent, :19552)
```

PilotDeck 前端的 `pilotdeck-bridge.js` 通过环境变量 `PILOTDECK_GATEWAY_URL` 配置要连接的 WebSocket 网关。我们只需：
1. HermesDeck Go 端实现 PilotDeck Gateway WebSocket 协议
2. 前端 bridge 改连 HermesDeck

## 背景：PilotDeck Gateway 协议

来自 `src/gateway/protocol/frames.ts`：

### WebSocket 帧类型

```
hello         → 客户端握手 { type, protocolVersion, clientName, token }
hello_ok      → 服务端回复 { type, protocolVersion, serverInfo }
request       → 客户端 RPC 调用 { type, id, method, params }
response      → 服务端回复 { type, id, ok, result|error }
event         → 服务端推送事件 { type, id, seq, final, event }
notification  → 服务端通知 { type, name, payload }
```

### 核心方法: `submit_turn`

```
request { method: "submit_turn", params: { sessionKey, channelKey, message, ... } }
  → event: { type: "turn_started", runId }
  → event: { type: "assistant_text_delta", text: "..." }
  → event: { type: "tool_call_started", toolCallId, name, argsPreview }
  → event: { type: "tool_call_finished", toolCallId, ok, resultPreview }
  → event: { type: "turn_completed", usage, finishReason }
  → response { ok: true, result: ... }
```

### 其他方法

`abort_turn`, `new_session`, `resume_session`, `close_session`, `list_sessions`, `describe_server`, `health_check`

## 实施方案

### 步骤 1: 添加 WebSocket 依赖

在 `src/go/go.mod` 中添加 `github.com/gorilla/websocket v1.5.3`。

### 步骤 2: 重写 `web.go` → 替换为 WebSocket Gateway

将 `src/go/internal/channel/web.go` 改写成 WebSocket Gateway 服务器：

**保留现有 REST 端点**（便于调试）：
- `GET /api/health` → 健康检查
- `GET /api/sessions` → 会话列表
- `GET /api/tools` → 工具列表

**新增 WebSocket 端点**：
- `GET /ws` → WebSocket 网关入口

**WebSocket 协议处理**（`internal/gateway/protocol.go`）：

```go
// 帧定义
type WsFrame struct {
    Type   string          `json:"type"`
    ID     string          `json:"id,omitempty"`
    Method string          `json:"method,omitempty"`
    Params json.RawMessage `json:"params,omitempty"`
    Ok     *bool           `json:"ok,omitempty"`
    Result interface{}     `json:"result,omitempty"`
    Error  *WsError        `json:"error,omitempty"`
    Event  json.RawMessage `json:"event,omitempty"`
    Seq    int             `json:"seq,omitempty"`
    Final  bool            `json:"final,omitempty"`
}
```

**会话管理器**（`internal/gateway/session.go`）：
- 维护 WebSocket 连接池
- 每个 `sessionKey` 对应一个 gRPC 会话
- 跟踪活跃的 `submit_turn` 运行

### 步骤 3: 实现 `submit_turn` 核心流程

```
WebSocket request submit_turn
  → 生成 runId
  → 发送 event: turn_started
  → 通过 gRPC ProcessMessage 调用 Python sidecar
  → 对每个 gRPC MessageResponse:
    → 如果包含 text → 发送 event: assistant_text_delta
    → 如果包含 tool_call → 发送 event: tool_call_started → tool_call_finished
    → 如果 is_final → 发送 event: turn_completed + response
```

**GatewayEvent 与 gRPC MessageResponse 的映射**：

| gRPC MessageResponse | GatewayEvent |
|---|---|
| `text` 且 `!is_final` | `assistant_text_delta { text }` |
| `text` 且 `is_final` | `turn_completed { usage, finishReason }` |
| `tool_call` | `tool_call_started` + `tool_call_finished` |
| `error` | `error { message, recoverable: false }` |

### 步骤 4: 处理 auth/握手

在 `hello` 帧中检查 token：
- 支持无 token 模式（开发环境）
- 可选验证 `~/.pilotdeck/gateway-token`

### 步骤 5: 处理其他 RPC 方法

- `new_session` → 创建 gRPC 会话
- `describe_server` → 返回 serverInfo
- `abort_turn` → 取消 gRPC stream
- `list_sessions` → 列出当前活跃会话

### 步骤 6: 配置 PilotDeck 前端

修改 PilotDeck 前端的启动命令，将 `PILOTDECK_GATEWAY_URL` 指向 HermesDeck：

```bash
# 原来
PILOTDECK_GATEWAY_URL=ws://127.0.0.1:18788/ws

# 改为
PILOTDECK_GATEWAY_URL=ws://127.0.0.1:18988/ws
```

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `src/go/go.mod` | 修改 | 添加 `gorilla/websocket` |
| `src/go/go.sum` | 自动更新 | |
| `src/go/internal/channel/web.go` | **重写** | 从内嵌 HTML 改为 WebSocket Gateway |
| `src/go/internal/gateway/protocol.go` | **新建** | WebSocket 帧定义 + 编解码 |
| `src/go/internal/gateway/session.go` | **新建** | WebSocket 会话管理 |
| `src/go/internal/gateway/handler.go` | **新建** | submit_turn 等 RPC 处理 |

## 不修改的文件

- `src/go/internal/bridge/` — gRPC 客户端/服务端不变
- `src/go/cmd/hermesdeck/main.go` — 启动逻辑不变
- `src/python/` — Python 端不变
- PilotDeck ui/server/index.js — 前端服务不变
- PilotDeck ui/server/pilotdeck-bridge.js — 只需改环境变量

## 测试验证

1. 启动 Python sidecar: `python3 -m hermesdeck_sidecar --port 19552`  # 默认端口已改为 19552
2. 启动 HermesDeck Go: `./bin/hermesdeck -listen=:19551 -web=:18988 -bridge=localhost:19552 -channel=web`
3. 用 `websocat` 或浏览器连接 `ws://localhost:18988/ws`
4. 发送 `hello` 帧验证握手
5. 发送 `submit_turn` 请求验证流式回复
6. 改 PilotDeck 前端的 `PILOTDECK_GATEWAY_URL` 指向 `ws://localhost:18988/ws`
7. 打开 PilotDeck 前端 UI 发送消息验证端到端

## 风险与注意事项

1. **Token 认证**：PilotDeck gateway 默认需要 token 认证（`~/.pilotdeck/gateway-token`）。HermesDeck 在开发模式应支持无 token 连接
2. **CORS**：前端 bridge 会跨端口访问，需要确保 CORS 头正确设置
3. **流式兼容性**：Hermes Agent 目前是单次回复（非流式），需要先在 Python 端改为 streaming 输出，或者 Go 端做缓冲再分段发送
4. **gRPC 错误处理**：WebSocket 连接断开时需要取消 gRPC stream 并清理资源
5. **PilotDeck 前端现有功能**：非聊天功能（项目管理、技能管理、文件浏览等）仍然通过 PilotDeck 前端的 REST API 处理，不受影响
