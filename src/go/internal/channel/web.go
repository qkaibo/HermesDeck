package channel

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/hermesdeck/internal/gateway"
	pb "github.com/hermesdeck/pkg/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // allow all origins for dev
}

type WebChannel struct {
	cfg        Config
	sessionMgr *gateway.GatewaySessionManager
}

func (c *WebChannel) Name() string { return "web" }

func (c *WebChannel) Run(ctx context.Context) error {
	c.sessionMgr = gateway.NewGatewaySessionManager()

	// Inject the gRPC client into the gateway package so handlers can use it.
	gateway.GetPBClient = func() pb.HermesDeckBridgeClient {
		return c.cfg.PyBridge.GetClient()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", c.handleHealth)
	mux.HandleFunc("/api/sessions", c.handleSessions)
	mux.HandleFunc("/api/tools", c.handleTools)
	mux.HandleFunc("/ws", c.handleWS)
	mux.HandleFunc("/", c.handleIndex)

	server := &http.Server{Addr: c.cfg.WebAddr, Handler: mux}
	log.Printf("Web channel (Gateway) listening on %s", c.cfg.WebAddr)
	go func() { <-ctx.Done(); server.Shutdown(context.Background()) }()
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ──────────────────────────────────────────────────────────────
//  WebSocket Gateway
// ──────────────────────────────────────────────────────────────

func (c *WebChannel) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[gateway] Upgrade error: %v", err)
		return
	}

	gs := gateway.NewGatewaySession(conn, "", "")
	c.sessionMgr.Add(gs)

	log.Printf("[gateway] WebSocket connection established from %s", r.RemoteAddr)

	defer func() {
		c.sessionMgr.Remove(conn)
		log.Printf("[gateway] WebSocket connection closed: %s", r.RemoteAddr)
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		gateway.HandleMessage(r.Context(), gs, message)
	}
}

// ──────────────────────────────────────────────────────────────
//  REST API endpoints
// ──────────────────────────────────────────────────────────────

func (c *WebChannel) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(indexHTML))
}

func (c *WebChannel) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "hermesdeck-go"})
}

func (c *WebChannel) handleSessions(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(c.cfg.SessionMgr.List())
}

func (c *WebChannel) handleTools(w http.ResponseWriter, r *http.Request) {
	type toolView struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	tools := c.cfg.ToolReg.List()
	views := make([]toolView, len(tools))
	for i, t := range tools {
		views[i] = toolView{Name: t.Name, Description: t.Description}
	}
	json.NewEncoder(w).Encode(views)
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>HermesDeck Gateway</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0d1117;color:#c9d1d9;display:flex;flex-direction:column;align-items:center;justify-content:center;height:100vh;gap:24px}
h1{color:#58a6ff;font-size:24px}
p{color:#8b949e;text-align:center;max-width:500px;line-height:1.6}
code{background:#21262d;padding:2px 8px;border-radius:4px;font-size:14px;color:#f0f6fc}
.status{display:flex;gap:16px}
.status span{padding:8px 16px;border-radius:6px;background:#21262d;border:1px solid #30363d}
.footer{color:#484f58;font-size:12px;margin-top:24px}
</style></head>
<body>
<h1>⚡ HermesDeck Gateway</h1>
<p>This is the HermesDeck WebSocket Gateway server.<br>
PilotDeck frontend connects via <code>/ws</code> endpoint.<br>
API endpoints: <code>/api/health</code> <code>/api/sessions</code> <code>/api/tools</code></p>
<div class="status">
<span>WS: <code>/ws</code></span>
<span>gRPC: <code>:29551</code></span>
<span>WS: <code>:28788</code></span>
<span>Proto: v1.0.0</span>
</div>
<div class="footer">HermesDeck — PilotDeck × Hermes Agent fusion</div>
</body></html>`
