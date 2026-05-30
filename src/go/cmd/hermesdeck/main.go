package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hermesdeck/internal/bridge"
	"github.com/hermesdeck/internal/channel"
	"github.com/hermesdeck/internal/session"
	"github.com/hermesdeck/internal/task"
	"github.com/hermesdeck/internal/tools"
)

var (
	bridgeAddr  = flag.String("bridge", "localhost:29552", "Python sidecar gRPC address")
	listenAddr  = flag.String("listen", ":29551", "Go runtime gRPC listen address")
	webAddr     = flag.String("web", ":28788", "Web channel listen address")
	channelMode = flag.String("channel", "cli", "Channel mode: cli, web, tui")
	pilotHome   = flag.String("pilot-home", "", "PilotDeck data directory (default: ~/.pilotdeck)")
)

func main() {
	flag.Parse()

	// Set PILOT_HOME env var so all internal code can find the data dir
	if *pilotHome != "" {
		os.Setenv("PILOT_HOME", *pilotHome)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println(">>> HermesDeck Go Runtime starting...")

	// 1. 初始化工具注册中心
	toolRegistry := tools.NewRegistry()
	registerBuiltinTools(toolRegistry)

	// 2. 初始化会话管理器
	sessionMgr := session.NewManager()

	// 3. 初始化后台任务系统
	taskMgr := task.NewManager()
	defer taskMgr.StopAll()

	// 4. 初始化 gRPC 桥接
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pyBridge := bridge.NewClient(*bridgeAddr, toolRegistry)
	if err := pyBridge.Connect(ctx); err != nil {
		log.Printf("WARNING: Cannot connect to Python sidecar at %s: %v", *bridgeAddr, err)
		log.Println("Running in local-only mode (no Hermes agent available).")
	} else {
		log.Printf("Connected to Python sidecar at %s", *bridgeAddr)

		// 向 Python 注册工具
		if err := pyBridge.RegisterTools(ctx, toolRegistry.List()); err != nil {
			log.Printf("WARNING: Failed to register tools: %v", err)
		} else {
			log.Printf("Registered %d tools with Python sidecar.", toolRegistry.Count())
		}
	}

	// 5. 启动 gRPC 服务端（让 Python 可以调用回来）
	goServer := bridge.NewServer(*listenAddr, toolRegistry, sessionMgr)
	go func() {
		if err := goServer.Start(ctx); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// 6. 启动用户通道
	ch, err := channel.NewChannel(*channelMode, channel.Config{
		WebAddr:    *webAddr,
		SessionMgr: sessionMgr,
		PyBridge:   pyBridge,
		ToolReg:    toolRegistry,
		TaskMgr:    taskMgr,
	})
	if err != nil {
		log.Fatalf("Cannot create channel '%s': %v", *channelMode, err)
	}

	// 7. 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// 运行通道（阻塞）
	if err := ch.Run(ctx); err != nil {
		log.Printf("Channel exited: %v", err)
	}

	log.Println("HermesDeck Go Runtime stopped.")
}

func registerBuiltinTools(r *tools.Registry) {
	// 文件系统工具
	r.Register(tools.ToolDefinition{
		Name:        "read_file",
		Description: "Read a file from the filesystem.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{"type": "string", "description": "Path to the file to read."},
			},
			"required": []string{"file_path"},
		},
		Handler: tools.ReadFileHandler,
	})

	r.Register(tools.ToolDefinition{
		Name:        "write_file",
		Description: "Write content to a file.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{"type": "string", "description": "Path to the file to write."},
				"content":   map[string]interface{}{"type": "string", "description": "Content to write."},
			},
			"required": []string{"file_path", "content"},
		},
		Handler: tools.WriteFileHandler,
	})

	r.Register(tools.ToolDefinition{
		Name:        "bash",
		Description: "Execute a shell command.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command":     map[string]interface{}{"type": "string", "description": "Shell command to execute."},
				"description": map[string]interface{}{"type": "string", "description": "What this command does."},
				"timeout":     map[string]interface{}{"type": "integer", "description": "Timeout in ms."},
			},
			"required": []string{"command"},
		},
		Handler: tools.BashHandler,
	})

	r.Register(tools.ToolDefinition{
		Name:        "grep",
		Description: "Search file contents using regular expressions.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern":     map[string]interface{}{"type": "string", "description": "Regex pattern to search."},
				"path":        map[string]interface{}{"type": "string", "description": "Directory or file to search."},
				"output_mode": map[string]interface{}{"type": "string", "description": "content / files_with_matches / count"},
			},
			"required": []string{"pattern"},
		},
		Handler: tools.GrepHandler,
	})

	r.Register(tools.ToolDefinition{
		Name:        "glob",
		Description: "Match files by glob pattern.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{"type": "string", "description": "Glob pattern."},
				"path":    map[string]interface{}{"type": "string", "description": "Search directory."},
			},
			"required": []string{"pattern"},
		},
		Handler: tools.GlobHandler,
	})

	r.Register(tools.ToolDefinition{
		Name:        "web_search",
		Description: "Search the web for current information.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "Search query."},
			},
			"required": []string{"query"},
		},
		Handler: tools.WebSearchHandler,
	})

	r.Register(tools.ToolDefinition{
		Name:        "web_fetch",
		Description: "Fetch and extract content from a URL.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":    map[string]interface{}{"type": "string", "description": "URL to fetch."},
				"prompt": map[string]interface{}{"type": "string", "description": "What to extract."},
			},
			"required": []string{"url"},
		},
		Handler: tools.WebFetchHandler,
	})
}
