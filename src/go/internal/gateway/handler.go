package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	pb "github.com/hermesdeck/pkg/proto"
)

// GetPBClient is a global hook injected by the web channel to provide
// the gRPC HermesDeckBridgeClient for submitting turns.
var GetPBClient func() pb.HermesDeckBridgeClient

// HandleMessage processes a single incoming WebSocket frame.
func HandleMessage(ctx context.Context, gs *GatewaySession, raw []byte) {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		sendError(gs, "", "parse_error", "invalid JSON")
		return
	}

	switch base.Type {
	case "hello":
		handleHello(gs, raw)
	case "request":
		handleRequest(ctx, gs, raw)
	default:
		sendError(gs, "", "unknown_type", "unknown frame type: "+base.Type)
	}
}

func handleHello(gs *GatewaySession, raw []byte) {
	var hello WsClientHello
	if err := json.Unmarshal(raw, &hello); err != nil {
		sendError(gs, "", "parse_error", "invalid hello frame")
		return
	}

	log.Printf("[gateway] Client '%s' (v%s) connected", hello.ClientName, hello.ClientVersion)
	gs.ClientName = hello.ClientName
	gs.Token = hello.Token

	resp := WsHelloOk{
		Type:            "hello_ok",
		ProtocolVersion: ProtocolVersion,
		ServerVersion:   ServerVersion,
		ServerInfo: WsServerInfo{
			Mode:            "remote",
			ProtocolVersion: ProtocolVersion,
			SessionCount:    len(gs.Sessions),
		},
	}
	gs.SendJSON(resp)
}

func handleRequest(ctx context.Context, gs *GatewaySession, raw []byte) {
	var req WsRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		sendError(gs, "", "parse_error", "invalid request frame")
		return
	}

	log.Printf("[gateway] Request: %s (id=%s)", req.Method, req.ID)

	switch req.Method {
	// ── Core chat ──
	case "submit_turn":
		handleSubmitTurn(ctx, gs, req)
	case "abort_turn":
		handleAbortTurn(gs, req)
	case "new_session":
		handleNewSession(gs, req)
	case "resume_session":
		handleResumeSession(gs, req)
	case "close_session":
		handleCloseSession(gs, req)
	case "list_sessions":
		handleListSessions(gs, req)
	case "describe_server":
		handleDescribeServer(gs, req)
	case "active_turn_snapshot":
		handleStub(gs, req, map[string]interface{}{})
	case "read_session_messages":
		handleStub(gs, req, map[string]interface{}{
			"messages": []interface{}{},
			"total":    0, "hasMore": false,
		})

	// ── Projects ──
	case "list_projects":
		handleListProjects(gs, req)
	case "describe_project":
		handleStub(gs, req, map[string]interface{}{})

	// ── Skills ──
	case "skill_list":
		handleSkillList(gs, req)
	case "skill_read":
		handleSkillRead(gs, req)
	case "skill_write":
		handleSkillWrite(gs, req)
	case "skill_create":
		handleSkillCreate(gs, req)
	case "skill_delete":
		handleSkillDelete(gs, req)
	case "skill_import":
		handleSkillImport(gs, req)
	case "skill_validate":
		handleSkillValidate(gs, req)
	case "skill_scan":
		handleSkillScan(gs, req)

	// ── Cron ──
	case "cron_create":
		handleStub(gs, req, map[string]interface{}{"id": "", "created": true})
	case "cron_list":
		handleStub(gs, req, map[string]interface{}{"crons": []interface{}{}})
	case "cron_delete":
		handleStub(gs, req, map[string]interface{}{"deleted": true})
	case "cron_stop":
		handleStub(gs, req, map[string]interface{}{"stopped": true})
	case "cron_run_now":
		handleStub(gs, req, map[string]interface{}{"started": true})

	// ── Permission / Elicitation ──
	case "elicitation_respond":
		handleStub(gs, req, map[string]interface{}{"delivered": true})
	case "permission_decide":
		handleStub(gs, req, map[string]interface{}{"delivered": true})
	case "grant_session_permission":
		handleStub(gs, req, map[string]interface{}{"granted": true})

	// ── Config / Always-on ──
	case "reload_config":
		handleStub(gs, req, map[string]interface{}{"reloaded": true})
	case "always_on_apply":
		handleStub(gs, req, map[string]interface{}{"applied": true})
	case "always_on_rerun_plan":
		handleStub(gs, req, map[string]interface{}{"started": true})

	default:
		sendResponse(gs, req.ID, false, &WsError{
			Code:    "unknown_method",
			Message: fmt.Sprintf("unknown method: %s", req.Method),
		})
	}
}

// ── Stub handler — returns empty/safe data for unimplemented methods ──

func handleStub(gs *GatewaySession, req WsRequest, result interface{}) {
	sendResponse(gs, req.ID, true, result)
}

// ── submit_turn — the core chat method ──

func handleSubmitTurn(ctx context.Context, gs *GatewaySession, req WsRequest) {
	var params SubmitTurnParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{
			Code: "invalid_params", Message: "cannot parse submit_turn params",
		})
		return
	}

	runID := params.RunID
	if runID == "" {
		runID = uuid.New().String()
	}

	// Emit turn_started
	seq := gs.NextSeq()
	gs.SendJSON(WsEvent{
		Type: "event", ID: req.ID, Seq: seq, Final: false,
		Event: MarshalEvent(TurnStartedEvent{Type: "turn_started", RunID: runID}),
	})

	// Create a cancellable context for this turn
	turnCtx, turnCancel := context.WithCancel(ctx)
	gs.GetOrCreatePBSession(params.SessionKey, turnCancel)

	// Get the gRPC client
	client := GetPBClient()
	if client == nil {
		sendErrorEvent(gs, req.ID, "Python sidecar not connected", false)
		sendResponse(gs, req.ID, false, &WsError{
			Code: "service_unavailable", Message: "Python sidecar not connected",
		})
		turnCancel()
		return
	}

	// Call gRPC ProcessMessage (streaming)
	stream, err := client.ProcessMessage(turnCtx, &pb.MessageRequest{
		SessionId:   params.SessionKey,
		UserMessage: params.Message,
		Stream:      true,
	})
	if err != nil {
		sendErrorEvent(gs, req.ID, fmt.Sprintf("gRPC error: %v", err), false)
		sendResponse(gs, req.ID, false, &WsError{
			Code: "grpc_error", Message: err.Error(),
		})
		turnCancel()
		return
	}

	// Stream responses from gRPC → GatewayEvents
	var fullText string
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}

		// Handle text content
		if text := resp.GetText(); text != "" {
			seq = gs.NextSeq()
			fullText += text
			gs.SendJSON(WsEvent{
				Type: "event", ID: req.ID, Seq: seq, Final: false,
				Event: MarshalEvent(AssistantTextDeltaEvent{
					Type: "assistant_text_delta",
					Text: text,
				}),
			})
		}

		// Handle tool calls
		if tc := resp.GetToolCall(); tc != nil {
			seq = gs.NextSeq()
			gs.SendJSON(WsEvent{
				Type: "event", ID: req.ID, Seq: seq, Final: false,
				Event: MarshalEvent(ToolCallStartedEvent{
					Type:       "tool_call_started",
					ToolCallID: tc.ToolCallId,
					Name:       tc.ToolName,
				}),
			})
			seq = gs.NextSeq()
			gs.SendJSON(WsEvent{
				Type: "event", ID: req.ID, Seq: seq, Final: false,
				Event: MarshalEvent(ToolCallFinishedEvent{
					Type:       "tool_call_finished",
					ToolCallID: tc.ToolCallId,
					Ok:         true,
				}),
			})
		}

		if resp.IsFinal {
			break
		}
	}

	// Emit turn_completed
	seq = gs.NextSeq()
	gs.SendJSON(WsEvent{
		Type: "event", ID: req.ID, Seq: seq, Final: true,
		Event: MarshalEvent(TurnCompletedEvent{
			Type: "turn_completed",
			Usage: TurnUsage{
				InputTokens:  0,
				OutputTokens: 0,
				TotalTokens:  0,
			},
			FinishReason: "stop",
		}),
	})

	// Send final response
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"runId": runID,
		"reply": fullText,
	})

	turnCancel()
}

// ── Other RPC handlers ──

func handleAbortTurn(gs *GatewaySession, req WsRequest) {
	var params AbortTurnParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse abort_turn params"})
		return
	}
	gs.CancelSession(params.SessionKey)
	sendResponse(gs, req.ID, true, map[string]interface{}{"cancelled": true})
}

func handleNewSession(gs *GatewaySession, req WsRequest) {
	var params NewSessionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse new_session params"})
		return
	}
	sessionKey := uuid.New().String()
	gs.GetOrCreatePBSession(sessionKey, nil)
	sendResponse(gs, req.ID, true, map[string]interface{}{"sessionKey": sessionKey})
}

func handleResumeSession(gs *GatewaySession, req WsRequest) {
	var params struct {
		SessionKey string `json:"sessionKey"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse resume_session params"})
		return
	}
	gs.GetOrCreatePBSession(params.SessionKey, nil)
	sendResponse(gs, req.ID, true, map[string]interface{}{"sessionKey": params.SessionKey})
}

func handleCloseSession(gs *GatewaySession, req WsRequest) {
	var params CloseSessionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse close_session params"})
		return
	}
	gs.RemovePBSession(params.SessionKey)
	sendResponse(gs, req.ID, true, map[string]interface{}{"closed": true})
}

func handleListSessions(gs *GatewaySession, req WsRequest) {
	gs.mu.Lock()
	sessions := make([]map[string]interface{}, 0, len(gs.Sessions))
	for k := range gs.Sessions {
		sessions = append(sessions, map[string]interface{}{
			"sessionKey": k,
		})
	}
	gs.mu.Unlock()
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"sessions": sessions,
	})
}

func handleListProjects(gs *GatewaySession, req WsRequest) {
	pilotHome := resolvePilotHome()
	projectsDir := pilotHome + "/projects"
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		sendResponse(gs, req.ID, true, map[string]interface{}{
			"projects": []interface{}{},
		})
		return
	}

	type projectEntry struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		FullPath    string `json:"fullPath"`
		Path        string `json:"path"`
	}
	projects := make([]projectEntry, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		realPath := ""
		cwdFile := projectsDir + "/" + e.Name() + "/.cwd"
		if data, err := os.ReadFile(cwdFile); err == nil {
			realPath = strings.TrimSpace(string(data))
		}
		displayName := e.Name()
		if strings.HasPrefix(displayName, "home-ts-") {
			displayName = strings.TrimPrefix(displayName, "home-ts-")
		}
		projects = append(projects, projectEntry{
			Name:        e.Name(),
			DisplayName: displayName,
			FullPath:    realPath,
			Path:        realPath,
		})
	}
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"projects": projects,
	})
}

func resolvePilotHome() string {
	pilotHome := os.Getenv("PILOT_HOME")
	if pilotHome == "" {
		home, _ := os.UserHomeDir()
		pilotHome = home + "/.pilotdeck"
	}
	return pilotHome
}

func handleDescribeServer(gs *GatewaySession, req WsRequest) {
	sendResponse(gs, req.ID, true, WsServerInfo{
		Mode:            "remote",
		ProtocolVersion: ProtocolVersion,
		SessionCount:    len(gs.Sessions),
	})
}

// ── Helpers ──

func sendResponse(gs *GatewaySession, id string, ok bool, resultOrError interface{}) {
	if ok {
		gs.SendJSON(WsResponse{Type: "response", ID: id, Ok: true, Result: resultOrError})
	} else {
		err, _ := resultOrError.(*WsError)
		gs.SendJSON(WsResponse{Type: "response", ID: id, Ok: false, Error: err})
	}
}

func sendError(gs *GatewaySession, id, code, message string) {
	gs.SendJSON(WsResponse{
		Type: "response", ID: id, Ok: false,
		Error: &WsError{Code: code, Message: message},
	})
}

func sendErrorEvent(gs *GatewaySession, id, message string, recoverable bool) {
	seq := gs.NextSeq()
	gs.SendJSON(WsEvent{
		Type: "event", ID: id, Seq: seq, Final: true,
		Event: MarshalEvent(ErrorEvent{
			Type:       "error",
			Message:    message,
			Code:       "internal_error",
			Recoverable: recoverable,
		}),
	})
}
