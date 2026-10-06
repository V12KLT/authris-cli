package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var agentRoot string

func lockAgentRoot() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}
	agentRoot = abs
	return nil
}

func insideRoot(path string) (string, error) {
	if agentRoot == "" {
		return "", fmt.Errorf("agent root not set")
	}
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == "" {
		return agentRoot, nil
	}
	if filepath.IsAbs(clean) {
		abs := clean
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		if abs != agentRoot && !strings.HasPrefix(abs, agentRoot+string(os.PathSeparator)) {
			return "", fmt.Errorf("outside project directory")
		}
		return abs, nil
	}
	joined := filepath.Join(agentRoot, clean)
	if resolved, err := filepath.EvalSymlinks(joined); err == nil {
		joined = resolved
	} else if parent, err := filepath.EvalSymlinks(filepath.Dir(joined)); err == nil {
		joined = filepath.Join(parent, filepath.Base(joined))
	}
	if joined != agentRoot && !strings.HasPrefix(joined, agentRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("outside project directory")
	}
	return joined, nil
}

func toolString(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func toolArg(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	lowered := map[string]string{}
	for key, value := range args {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			lowered[strings.ToLower(key)] = text
		}
	}
	for _, key := range keys {
		if text, ok := lowered[strings.ToLower(key)]; ok {
			return text
		}
	}
	return ""
}

func argKeys(args map[string]any) string {
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "(none)"
	}
	return strings.Join(keys, ", ")
}

func toolPath(args map[string]any) string {
	return toolArg(args, "path", "file", "filename", "filepath", "dir", "folder")
}

func toolFSList(args map[string]any) (string, error) {
	dir, err := insideRoot(toolPath(args))
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 200 {
		names = names[:200]
	}
	if len(names) == 0 {
		return "(empty)", nil
	}
	return strings.Join(names, "\n"), nil
}

func toolFSRead(args map[string]any) (string, error) {
	path := strings.TrimSpace(toolPath(args))
	if path == "" {
		return "", fmt.Errorf("path is required (got: %s)", argKeys(args))
	}
	full, err := insideRoot(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("is a directory")
	}
	if info.Size() > 256*1024 {
		return "", fmt.Errorf("file too large")
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	text := string(raw)
	if len(text) > 64*1024 {
		text = text[:64*1024] + "\n(truncated)"
	}
	return text, nil
}

func toolFSWrite(args map[string]any) (string, error) {
	path := strings.TrimSpace(toolPath(args))
	if path == "" {
		return "", fmt.Errorf("path is required (got: %s)", argKeys(args))
	}
	full, err := insideRoot(path)
	if err != nil {
		return "", err
	}
	content := toolArg(args, "content", "text", "data", "body")
	if len(content) > 512*1024 {
		return "", fmt.Errorf("content too large")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", err
	}
	rel, _ := filepath.Rel(agentRoot, full)
	return "wrote " + rel, nil
}

func toolFSEdit(args map[string]any) (string, error) {
	path := strings.TrimSpace(toolPath(args))
	oldText := toolArg(args, "old_text", "old", "find", "search")
	newText := toolArg(args, "new_text", "new", "replace", "replacement")
	if path == "" || oldText == "" {
		return "", fmt.Errorf("path and old_text are required (got: %s)", argKeys(args))
	}
	full, err := insideRoot(path)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if strings.Count(string(raw), oldText) != 1 {
		return "", fmt.Errorf("match is not unique")
	}
	updated := strings.Replace(string(raw), oldText, newText, 1)
	if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
		return "", err
	}
	rel, _ := filepath.Rel(agentRoot, full)
	return "edited " + rel, nil
}

func toolExec(args map[string]any) (string, error) {
	command := strings.TrimSpace(toolArg(args, "command", "cmd", "script"))
	if command == "" {
		return "", fmt.Errorf("command is required (got: %s)", argKeys(args))
	}
	if len(command) > 4000 {
		return "", fmt.Errorf("command too long")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if os.Getenv("OS") == "Windows_NT" || filepath.Separator == '\\' {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = agentRoot
	raw, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(raw))
	if len(text) > 16*1024 {
		text = text[:16*1024] + "\n(truncated)"
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("timed out after 300s: %s", text)
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("exit failed: %s", text)
	}
	if text == "" {
		return "ok", nil
	}
	return text, nil
}

func runLocalTool(name string, args map[string]any) (string, error) {
	switch name {
	case "fs_list":
		return toolFSList(args)
	case "fs_read":
		return toolFSRead(args)
	case "fs_write":
		return toolFSWrite(args)
	case "fs_edit":
		return toolFSEdit(args)
	case "exec":
		return toolExec(args)
	}
	return "", fmt.Errorf("unknown tool")
}

func localCallSummary(name string, args map[string]any) string {
	path := strings.TrimSpace(toolPath(args))
	switch name {
	case "fs_list":
		if path == "" {
			return "list project files"
		}
		return "list " + path
	case "fs_read":
		return "read " + path
	case "fs_write":
		return fmt.Sprintf("write %s (%d bytes)", path, len(toolString(args, "content")))
	case "fs_edit":
		return "edit " + path
	case "exec":
		command := strings.TrimSpace(toolString(args, "command"))
		if len(command) > 120 {
			command = command[:120] + "…"
		}
		return "run: " + command
	}
	return name
}
