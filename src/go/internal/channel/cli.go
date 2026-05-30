package channel

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	pb "github.com/hermesdeck/pkg/proto"
	"github.com/hermesdeck/internal/bridge"
	"github.com/hermesdeck/internal/session"
	"github.com/hermesdeck/internal/task"
	"github.com/hermesdeck/internal/tools"
)

type Config struct {
	WebAddr    string
	SessionMgr *session.Manager
	PyBridge   *bridge.Client
	ToolReg    *tools.Registry
	TaskMgr    *task.Manager
}

type Channel interface {
	Run(ctx context.Context) error
	Name() string
}

func NewChannel(mode string, cfg Config) (Channel, error) {
	switch mode {
	case "cli":
		return &CLIChannel{cfg: cfg}, nil
	case "web":
		return &WebChannel{cfg: cfg}, nil
	case "tui":
		return &TUIChannel{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unknown channel: %s", mode)
	}
}

// ================================================================
// CLI Channel
// ================================================================

type CLIChannel struct {
	cfg Config
}

func (c *CLIChannel) Name() string { return "cli" }

func (c *CLIChannel) Run(ctx context.Context) error {
	sess := c.cfg.SessionMgr.Create(c.cfg.SessionMgr.CreateID())
	fmt.Println("HermesDeck CLI")
	fmt.Println("Type /help for commands, /quit to exit.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			if !c.handleCommand(input, sess) {
				return nil
			}
			continue
		}

		// 处理用户消息
		c.handleMessage(ctx, sess, input)
	}

	return scanner.Err()
}

func (c *CLIChannel) handleCommand(input string, sess *session.Session) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit":
		fmt.Println("Goodbye!")
		return false

	case "/help":
		fmt.Println("Commands:")
		fmt.Println("  /quit       Exit")
		fmt.Println("  /help       Show this help")
		fmt.Println("  /mode       Show current session mode")
		fmt.Println("  /mode remote Switch to Hermes agent mode")
		fmt.Println("  /mode local  Switch to local tool mode")
		fmt.Println("  /session    Show session info")
		fmt.Println("  /tools      List available tools")
		return true

	case "/mode":
		if len(parts) >= 2 {
			switch parts[1] {
			case "remote":
				sess.Mode = session.ModeRemote
				fmt.Println("Switched to Hermes agent mode.")
			case "local":
				sess.Mode = session.ModeLocal
				fmt.Println("Switched to local tool mode.")
			default:
				fmt.Printf("Unknown mode: %s\n", parts[1])
			}
		} else {
			fmt.Printf("Current mode: %s\n", sess.Mode)
		}
		return true

	case "/session":
		fmt.Printf("Session ID: %s\n", sess.ID)
		fmt.Printf("Mode: %s\n", sess.Mode)
		fmt.Printf("Messages: %d\n", len(sess.GetHistory()))
		fmt.Printf("Hermes Session: %s\n", sess.HermesSessionID)
		return true

	case "/tools":
		tools := c.cfg.ToolReg.List()
		fmt.Printf("Available tools (%d):\n", len(tools))
		for _, t := range tools {
			fmt.Printf("  - %s: %s\n", t.Name, t.Description)
		}
		return true

	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		return true
	}
}

func (c *CLIChannel) handleMessage(ctx context.Context, sess *session.Session, input string) {
	// 添加到历史
	sess.AddMessage(session.ChatMessage{Role: "user", Content: input})

	if sess.Mode == session.ModeRemote && c.cfg.PyBridge.IsConnected() {
		// Hermes 模式
		respCh, err := c.cfg.PyBridge.ProcessMessage(ctx, sess.ID, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		fmt.Print("🤖 ")
		for resp := range respCh {
			switch v := resp.Content.(type) {
			case *pb.MessageResponse_Text:
				fmt.Print(v.Text)
			case *pb.MessageResponse_ToolCall:
				fmt.Printf("\n[Using tool: %s]\n", v.ToolCall.ToolName)
			case *pb.MessageResponse_Error:
				fmt.Printf("\n[Error: %s]\n", v.Error.Message)
			}
		}
		fmt.Println()
	} else {
		// 本地模式 — 直接执行命令
		fmt.Println("[Local mode] Execute with /mode remote for agent mode.")
	}
}
