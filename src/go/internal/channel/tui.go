package channel

import (
	"context"
	"fmt"
)

// TUIChannel 提供终端用户界面（简化版，实际可用 tcell/bubbletea）
type TUIChannel struct {
	cfg Config
}

func (c *TUIChannel) Name() string { return "tui" }

func (c *TUIChannel) Run(ctx context.Context) error {
	fmt.Println("TUI mode — requires tcell/bubbletea for full implementation.")
	fmt.Println("Falling back to CLI mode...")

	cli := &CLIChannel{cfg: c.cfg}
	return cli.Run(ctx)
}
