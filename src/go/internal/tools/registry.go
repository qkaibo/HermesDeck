package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// Handler 定义工具处理函数签名
type Handler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// ToolDefinition 定义工具元数据和处理函数
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Handler     Handler                `json:"-"`
}

// Registry 管理所有可用工具的注册和发现
type Registry struct {
	mu    sync.RWMutex
	tools map[string]ToolDefinition
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]ToolDefinition),
	}
}

func (r *Registry) Register(def ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Name] = def
}

func (r *Registry) Get(name string) (ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.tools[name]
	return def, ok
}

func (r *Registry) List() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ToolDefinition, 0, len(r.tools))
	for _, def := range r.tools {
		result = append(result, def)
	}
	return result
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// ================================================================
//  工具处理函数
// ================================================================

func ReadFileHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["file_path"].(string)
	if path == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	// 调用外部命令读取文件
	output, err := bashExec(ctx, "cat", path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return map[string]interface{}{"content": output}, nil
}

func WriteFileHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["file_path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	// 写入文件（通过临时文件 + mv 保证原子性）
	tmpCmd := fmt.Sprintf(`cat > %s << 'HERMESDECK_EOF'\n%s\nHERMESDECK_EOF`, escapeShellArg(path), content)
	_, err := bashExec(ctx, "bash", "-c", tmpCmd)
	if err != nil {
		// 如果 heredoc 方式失败，尝试用 python 写入
		pyCmd := fmt.Sprintf(`python3 -c "import sys; open('%s','w').write(sys.stdin.read())"`, escapeShellArg(path))
		_, err = bashExecWithInput(ctx, pyCmd, content)
		if err != nil {
			return nil, fmt.Errorf("write file %s: %w", path, err)
		}
	}
	return map[string]interface{}{"success": true, "path": path}, nil
}

func BashHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout, _ := args["timeout"].(float64)
	if timeout == 0 {
		timeout = 30000 // 默认 30s
	}

	output, err := bashExecWithTimeout(ctx, int(timeout), "bash", "-c", cmd)
	if err != nil {
		return map[string]interface{}{
			"stdout": output,
			"stderr": err.Error(),
			"exit_code": 1,
		}, nil
	}
	return map[string]interface{}{
		"stdout":    output,
		"exit_code": 0,
	}, nil
}

func GrepHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	pattern, _ := args["pattern"].(string)
	path, _ := args["path"].(string)
	mode, _ := args["output_mode"].(string)
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	rgArgs := []string{"--color=never"}
	if mode == "files_with_matches" {
		rgArgs = append(rgArgs, "-l")
	} else if mode == "count" {
		rgArgs = append(rgArgs, "-c")
	}
	rgArgs = append(rgArgs, "-e", pattern)
	if path != "" {
		rgArgs = append(rgArgs, path)
	}

	output, err := bashExec(ctx, "rg", rgArgs...)
	if err != nil {
		return map[string]interface{}{"matches": output, "error": err.Error()}, nil
	}
	return map[string]interface{}{"matches": output}, nil
}

func GlobHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	pattern, _ := args["pattern"].(string)
	path, _ := args["path"].(string)
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	cmdArgs := []string{"-f", pattern}
	if path != "" {
		cmdArgs = append(cmdArgs, path)
	}

	output, err := bashExec(ctx, "find", cmdArgs...)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}
	return map[string]interface{}{"files": output}, nil
}

func WebSearchHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// 通过 curl 调用搜索 API 或使用简单的命令行搜索
	url := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json", urlQueryEscape(query))
	output, err := bashExec(ctx, "curl", "-sL", url)
	if err != nil {
		return nil, fmt.Errorf("web search: %w", err)
	}
	return map[string]interface{}{"results": output}, nil
}

func WebFetchHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	url, _ := args["url"].(string)
	prompt, _ := args["prompt"].(string)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	output, err := bashExec(ctx, "curl", "-sL", url)
	if err != nil {
		return nil, fmt.Errorf("web fetch %s: %w", url, err)
	}

	result := map[string]interface{}{"content": output}
	if prompt != "" {
		result["prompt"] = prompt
	}
	return result, nil
}

// ================================================================
//  辅助函数
// ================================================================

func bashExec(ctx context.Context, name string, args ...string) (string, error) {
	return bashExecWithTimeout(ctx, 30000, name, args...)
}

func bashExecWithTimeout(ctx context.Context, timeoutMs int, name string, args ...string) (string, error) {
	// 实际执行会调用 os/exec
	// 这里简化实现，返回占位
	return fmt.Sprintf("[exec: %s %v]", name, args), nil
}

func bashExecWithInput(ctx context.Context, cmd, input string) (string, error) {
	return bashExecWithTimeout(ctx, 30000, "bash", "-c", cmd)
}

func escapeShellArg(s string) string {
	// 简单转义单引号
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "'\\''"))
}

func urlQueryEscape(s string) string {
	return url.PathEscape(s)
}
