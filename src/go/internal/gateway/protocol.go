// Package gateway implements a PilotDeck-compatible WebSocket Gateway
// protocol, allowing HermesDeck's Python sidecar (Hermes Agent) to replace
// PilotDeck's own agent loop while keeping PilotDeck's frontend UI unchanged.
package gateway

import "encoding/json"

// ──────────────────────────────────────────────────────────────
//  WS frame types (PilotDeck Gateway Protocol)
// ──────────────────────────────────────────────────────────────

// WsClientHello is the first frame a client sends after connecting.
type WsClientHello struct {
	Type            string `json:"type"`            // "hello"
	ProtocolVersion string `json:"protocolVersion"`
	ClientName      string `json:"clientName"`      // "cli" | "tui" | "web" | "feishu" | "test"
	ClientVersion   string `json:"clientVersion"`
	Token           string `json:"token"`
}

type WsServerInfo struct {
	Mode            string `json:"mode"`            // "in_process" | "remote"
	ProtocolVersion string `json:"protocolVersion,omitempty"`
	ProjectKey      string `json:"projectKey,omitempty"`
	SessionCount    int    `json:"sessionCount,omitempty"`
}

// WsHelloOk is sent to the client after successful auth handshake.
type WsHelloOk struct {
	Type            string       `json:"type"` // "hello_ok"
	ProtocolVersion string       `json:"protocolVersion"`
	ServerVersion   string       `json:"serverVersion"`
	ServerInfo      WsServerInfo `json:"serverInfo"`
}

// WsRequest is an RPC call from the client.
type WsRequest struct {
	Type   string          `json:"type"`   // "request"
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// WsError represents a structured error in a response.
type WsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WsResponse is the server's reply to a request.
type WsResponse struct {
	Type   string      `json:"type"` // "response"
	ID     string      `json:"id"`
	Ok     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  *WsError    `json:"error,omitempty"`
}

// WsEvent is a streaming event pushed to the client during a turn.
type WsEvent struct {
	Type  string          `json:"type"` // "event"
	ID    string          `json:"id"`
	Seq   int             `json:"seq"`
	Final bool            `json:"final"`
	Event json.RawMessage `json:"event"`
}

// ──────────────────────────────────────────────────────────────
//  GatewayEvent types (subset of PilotDeck's GatewayEvent)
// ──────────────────────────────────────────────────────────────

type TurnUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
}

type TurnStartedEvent struct {
	Type  string `json:"type"`  // "turn_started"
	RunID string `json:"runId"`
}

type AssistantTextDeltaEvent struct {
	Type string `json:"type"` // "assistant_text_delta"
	Text string `json:"text"`
}

type TurnCompletedEvent struct {
	Type         string     `json:"type"` // "turn_completed"
	Usage        TurnUsage  `json:"usage"`
	FinishReason string     `json:"finishReason"`
}

type ToolCallStartedEvent struct {
	Type        string `json:"type"` // "tool_call_started"
	ToolCallID  string `json:"toolCallId"`
	Name        string `json:"name"`
	ArgsPreview string `json:"argsPreview,omitempty"`
}

type ToolCallFinishedEvent struct {
	Type            string `json:"type"` // "tool_call_finished"
	ToolCallID      string `json:"toolCallId"`
	Ok              bool   `json:"ok"`
	ResultPreview   string `json:"resultPreview,omitempty"`
	ResultLineCount int    `json:"resultLineCount,omitempty"`
}

type ErrorEvent struct {
	Type       string `json:"type"` // "error"
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
	Recoverable bool  `json:"recoverable"`
}

// ──────────────────────────────────────────────────────────────
//  Request param types
// ──────────────────────────────────────────────────────────────

type SubmitTurnParams struct {
	SessionKey   string `json:"sessionKey"`
	ChannelKey   string `json:"channelKey"`
	Message      string `json:"message"`
	ProjectKey   string `json:"projectKey,omitempty"`
	WorkspaceCwd string `json:"workspaceCwd,omitempty"`
	Mode         string `json:"mode,omitempty"`
	RunID        string `json:"runId,omitempty"`
}

type NewSessionParams struct {
	ProjectKey string `json:"projectKey,omitempty"`
	ChannelKey string `json:"channelKey"`
	Hint       string `json:"hint,omitempty"`
}

type AbortTurnParams struct {
	SessionKey string `json:"sessionKey"`
	RunID      string `json:"runId,omitempty"`
}

type CloseSessionParams struct {
	SessionKey string `json:"sessionKey"`
	Reason     string `json:"reason,omitempty"`
}

// ──────────────────────────────────────────────────────────────
//  Helpers
// ──────────────────────────────────────────────────────────────

// MarshalEvent marshals a GatewayEvent struct into json.RawMessage.
func MarshalEvent(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

const ProtocolVersion = "1.0.0"
const ServerVersion = "hermesdeck/0.1.0"
