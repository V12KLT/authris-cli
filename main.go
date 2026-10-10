package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/chzyer/readline"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type credentials struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

func credPath() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "authris", "credentials.json")
}

func loadCreds() (credentials, error) {
	var creds credentials
	raw, err := os.ReadFile(credPath())
	if err != nil {
		return creds, err
	}
	if err := json.Unmarshal(raw, &creds); err != nil {
		return creds, err
	}
	if creds.Server == "" || creds.Token == "" {
		return creds, fmt.Errorf("incomplete credentials")
	}
	return creds, nil
}

func saveCreds(creds credentials) error {
	path := credPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func rpc(server string, token string, id any, method string, params any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	req, err := http.NewRequest("POST", strings.TrimSuffix(server, "/")+"/mcp", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("rejected (%d). Run `authris login` again", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}
	var parsed rpcResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("bad response: %v", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("%s", parsed.Error.Message)
	}
	return parsed.Result, nil
}

func toolText(result json.RawMessage) (string, bool, error) {
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return "", false, err
	}
	if len(out.Content) == 0 {
		return "", out.IsError, fmt.Errorf("empty tool result")
	}
	return out.Content[0].Text, out.IsError, nil
}

func callTool(creds credentials, name string, args map[string]any) (string, error) {
	raw, err := rpc(creds.Server, creds.Token, 1, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	text, isError, err := toolText(raw)
	if err != nil {
		return "", err
	}
	if isError {
		return "", fmt.Errorf("%s", text)
	}
	return text, nil
}

func cmdLogin(args []string) int {
	server := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--server" && i+1 < len(args) {
			server = args[i+1]
			i++
		}
	}
	if server == "" {
		fmt.Print("Server URL: ")
		fmt.Scanln(&server)
	}
	server = strings.TrimSpace(server)
	if server == "" {
		fmt.Println("server URL is required")
		return 1
	}
	fmt.Println("Paste a personal token from Dashboard > AI Access:")
	fmt.Print("Token: ")
	reader := bufio.NewReader(os.Stdin)
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(token)
	if token == "" {
		fmt.Println("token is required")
		return 1
	}
	if _, err := rpc(server, token, 1, "initialize", map[string]any{}); err != nil {
		fmt.Println("login failed:", err)
		return 1
	}
	if err := saveCreds(credentials{Server: server, Token: token}); err != nil {
		fmt.Println("could not save credentials:", err)
		return 1
	}
	fmt.Println("logged in to", server)
	return 0
}

func cmdStatus() int {
	creds, err := loadCreds()
	if err != nil {
		fmt.Println("not logged in. Run `authris login --server <url>`")
		return 1
	}
	raw, err := rpc(creds.Server, creds.Token, 1, "resources/read", map[string]any{"uri": "authris://server-info"})
	if err != nil {
		fmt.Println("status failed:", err)
		return 1
	}
	var contents struct {
		Contents []struct {
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &contents); err != nil || len(contents.Contents) == 0 {
		fmt.Println("status failed: bad resource")
		return 1
	}
	var info struct {
		BaseURL string   `json:"base_url"`
		Scopes  []string `json:"grant_scopes"`
		User    string   `json:"username"`
	}
	_ = json.Unmarshal([]byte(contents.Contents[0].Text), &info)
	rawTools, err := rpc(creds.Server, creds.Token, 2, "tools/list", map[string]any{})
	if err != nil {
		fmt.Println("status failed:", err)
		return 1
	}
	var tools struct {
		Tools []any `json:"tools"`
	}
	_ = json.Unmarshal(rawTools, &tools)
	fmt.Printf("server: %s\nuser: %s\nscopes: %s\ntools: %d\n", info.BaseURL, info.User, strings.Join(info.Scopes, " "), len(tools.Tools))
	return 0
}

func cmdKeys(args []string) int {
	creds, err := loadCreds()
	if err != nil {
		fmt.Println("not logged in. Run `authris login --server <url>`")
		return 1
	}
	project := ""
	if len(args) > 0 {
		project = args[0]
	}
	if project == "" {
		text, err := callTool(creds, "list_projects", map[string]any{})
		if err != nil {
			fmt.Println(err)
			return 1
		}
		var list struct {
			Projects []struct {
				ID   string `json:"project_id"`
				Name string `json:"name"`
			} `json:"projects"`
		}
		_ = json.Unmarshal([]byte(text), &list)
		if len(list.Projects) == 0 {
			fmt.Println("no projects")
			return 1
		}
		if len(list.Projects) > 1 {
			fmt.Println("projects:")
			for _, p := range list.Projects {
				fmt.Printf("  %s  %s\n", p.ID, p.Name)
			}
			fmt.Println("usage: authris keys <project-id-or-name>")
			return 1
		}
		project = list.Projects[0].ID
	}
	text, err := callTool(creds, "key_inventory", map[string]any{"project": project})
	if err != nil {
		fmt.Println(err)
		return 1
	}
	var inv struct {
		Inventory map[string]int `json:"inventory"`
		Used      int            `json:"quota_used"`
		Max       int            `json:"quota_max"`
	}
	_ = json.Unmarshal([]byte(text), &inv)
	order := []string{"total", "unused", "bound", "paused", "expired", "expiring_soon", "honeypots"}
	parts := make([]string, 0, len(order))
	for _, key := range order {
		if value, ok := inv.Inventory[key]; ok {
			parts = append(parts, fmt.Sprintf("%d %s", value, strings.ReplaceAll(key, "_", " ")))
		}
	}
	fmt.Println(strings.Join(parts, " · "))
	if inv.Max > 0 {
		fmt.Printf("quota: %d/%d\n", inv.Used, inv.Max)
	}
	return 0
}

func cmdCall(args []string) int {
	creds, err := loadCreds()
	if err != nil {
		fmt.Println("not logged in. Run `authris login --server <url>`")
		return 1
	}
	if len(args) == 0 {
		fmt.Println("usage: authris call <tool> [json-args]")
		return 1
	}
	toolArgs := map[string]any{}
	if len(args) > 1 {
		if err := json.Unmarshal([]byte(args[1]), &toolArgs); err != nil {
			fmt.Println("bad json args:", err)
			return 1
		}
	}
	text, err := callTool(creds, args[0], toolArgs)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	fmt.Println(text)
	return 0
}

func cmdMCP() int {
	creds, err := loadCreds()
	if err != nil {
		fmt.Fprintln(os.Stderr, "not logged in. Run `authris login --server <url>`")
		return 1
	}
	endpoint := strings.TrimSuffix(creds.Server, "/") + "/mcp"
	client := &http.Client{Timeout: 120 * time.Second}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		req, err := http.NewRequest("POST", endpoint, strings.NewReader(line))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+creds.Token)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusAccepted || len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			fmt.Fprintln(os.Stderr, "token rejected. Run `authris login` again")
			return 1
		}
		writer.Write(raw)
		if !bytes.HasSuffix(raw, []byte("\n")) {
			writer.WriteString("\n")
		}
		writer.Flush()
	}
	return 0
}

func chatRequest(creds credentials, payload map[string]any) (map[string]any, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", strings.TrimSuffix(creds.Server, "/")+"/mcp-ai/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("rejected (%d). Run `authris login` again", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusConflict {
		return nil, fmt.Errorf("enable the assistant from the dashboard first")
	}
	if resp.StatusCode != http.StatusOK {
		var failure map[string]any
		_ = json.Unmarshal(raw, &failure)
		if message, ok := failure["error"].(string); ok && message != "" {
			return nil, fmt.Errorf("%s", message)
		}
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("bad response: %v", err)
	}
	return parsed, nil
}

func defaultProject(creds credentials) string {
	text, err := callTool(creds, "list_projects", map[string]any{})
	if err != nil {
		return ""
	}
	var list struct {
		Projects []struct {
			ID string `json:"project_id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(text), &list); err != nil {
		return ""
	}
	if len(list.Projects) == 1 {
		return list.Projects[0].ID
	}
	return ""
}

func printKeySystemHeader(creds credentials, selected string) {
	text, err := callTool(creds, "list_projects", map[string]any{})
	if err != nil {
		return
	}
	var list struct {
		Projects []struct {
			ID     string `json:"project_id"`
			Name   string `json:"name"`
			Total  int    `json:"total_keys"`
			Active int    `json:"active"`
		} `json:"projects"`
	}
	if json.Unmarshal([]byte(text), &list) != nil {
		return
	}
	username := "connected account"
	if raw, err := rpc(creds.Server, creds.Token, 1, "resources/read", map[string]any{"uri": "authris://server-info"}); err == nil {
		var resource struct {
			Contents []struct {
				Text string `json:"text"`
			} `json:"contents"`
		}
		if json.Unmarshal(raw, &resource) == nil && len(resource.Contents) > 0 {
			var info struct {
				User string `json:"username"`
			}
			_ = json.Unmarshal([]byte(resource.Contents[0].Text), &info)
			if info.User != "" {
				username = info.User
			}
		}
	}
	fmt.Println()
	fmt.Println(bold(cyan("Authris key system")))
	fmt.Printf("%s %s  ·  %s %d\n", dim("account"), username, dim("projects"), len(list.Projects))
	for _, p := range list.Projects {
		if selected == "" || selected == p.ID || selected == p.Name {
			fmt.Printf("%s %s  ·  %d keys  ·  %d active\n", dim("project"), p.Name, p.Total, p.Active)
			break
		}
	}
	fmt.Println()
}

type slashCommand struct {
	Name        string
	Description string
}

var slashCommands = []slashCommand{
	{Name: "/project", Description: "switch the active project"},
	{Name: "/projects", Description: "list projects and key totals"},
	{Name: "/keys", Description: "show key inventory"},
	{Name: "/list-keys", Description: "list key prefixes"},
	{Name: "/add-key", Description: "create keys"},
	{Name: "/delete-key", Description: "delete a key"},
	{Name: "/pause-key", Description: "pause a key"},
	{Name: "/unpause-key", Description: "unpause a key"},
	{Name: "/extend-key", Description: "extend a key"},
	{Name: "/reset-hwid", Description: "reset a key HWID"},
	{Name: "/key-note", Description: "save a key note"},
	{Name: "/stats", Description: "show project stats"},
	{Name: "/setup", Description: "show integration setup"},
	{Name: "/status", Description: "show account connection"},
	{Name: "/call", Description: "call an Authris tool"},
	{Name: "/perms", Description: "show local permissions"},
	{Name: "/effort", Description: "change AI effort"},
	{Name: "/update", Description: "update the CLI"},
	{Name: "/help", Description: "show all commands"},
	{Name: "/quit", Description: "exit the CLI"},
}

func slashMatches(line string) []slashCommand {
	text := strings.TrimSpace(line)
	if !strings.HasPrefix(text, "/") {
		return nil
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}
	query := strings.ToLower(fields[0])
	matches := make([]slashCommand, 0, len(slashCommands))
	for _, command := range slashCommands {
		if query == "/" || strings.HasPrefix(command.Name, query) {
			matches = append(matches, command)
		}
	}
	return matches
}

func slashPrompt(line string) string {
	matches := slashMatches(line)
	if matches == nil {
		return "❯ "
	}
	var b strings.Builder
	if len(matches) == 0 {
		b.WriteString("  no matching commands\n")
	}
	visible := matches
	if len(visible) > 4 {
		visible = visible[:4]
	}
	for _, command := range visible {
		fmt.Fprintf(&b, "  %-14s %s\n", command.Name, command.Description)
	}
	for i := len(visible); i < 4; i++ {
		b.WriteString("\n")
	}
	b.WriteString("❯ ")
	return b.String()
}
func slashMenu() {
	fmt.Println(bold("Commands"))
	fmt.Println("  /project <name|id>       switch the active project")
	fmt.Println("  /projects                list projects and key totals")
	fmt.Println("  /keys                    show key inventory")
	fmt.Println("  /list-keys               list key prefixes")
	fmt.Println("  /add-key [count] [time]  create keys")
	fmt.Println("  /delete-key <key>        delete a key")
	fmt.Println("  /pause-key <key>         pause a key")
	fmt.Println("  /unpause-key <key>       unpause a key")
	fmt.Println("  /extend-key <key> <time> extend a key")
	fmt.Println("  /reset-hwid <key>        reset a key HWID")
	fmt.Println("  /key-note <key> <note>   save a key note")
	fmt.Println("  /stats                   show project stats")
	fmt.Println("  /setup <language>        show integration setup")
	fmt.Println("  /status                  show account connection")
	fmt.Println("  /call <tool> <json>      call an Authris tool")
	fmt.Println("  /perms, /effort, /update, /help, /quit")
}

func slashProjects(creds credentials) {
	text, err := callTool(creds, "list_projects", map[string]any{})
	if err != nil {
		fmt.Println(red(err.Error()))
		return
	}
	var list struct {
		Projects []struct {
			ID     string `json:"project_id"`
			Name   string `json:"name"`
			Total  int    `json:"total_keys"`
			Active int    `json:"active"`
		} `json:"projects"`
	}
	if json.Unmarshal([]byte(text), &list) != nil {
		fmt.Println(red("could not read projects"))
		return
	}
	for _, p := range list.Projects {
		fmt.Printf("  %s  %s  ·  %d keys  ·  %d active\n", p.ID, p.Name, p.Total, p.Active)
	}
	if len(list.Projects) == 0 {
		fmt.Println("  no projects")
	}
}

func slashProject(creds credentials, args []string, project *string) {
	if len(args) == 0 {
		slashProjects(creds)
		return
	}
	text, err := callTool(creds, "list_projects", map[string]any{})
	if err != nil {
		fmt.Println(red(err.Error()))
		return
	}
	var list struct {
		Projects []struct {
			ID   string `json:"project_id"`
			Name string `json:"name"`
		} `json:"projects"`
	}
	if json.Unmarshal([]byte(text), &list) != nil {
		fmt.Println(red("could not read projects"))
		return
	}
	needle := strings.Join(args, " ")
	for _, p := range list.Projects {
		if p.ID == needle || strings.EqualFold(p.Name, needle) {
			*project = p.ID
			fmt.Println("project -> " + p.Name)
			return
		}
	}
	fmt.Println(red("project not found: " + needle))
}

func slashRead(creds credentials, name string, project string, args map[string]any) {
	if project == "" {
		fmt.Println(red("choose a project first with /project <name|id>"))
		return
	}
	args["project"] = project
	text, err := callTool(creds, name, args)
	if err != nil {
		fmt.Println(red(err.Error()))
		return
	}
	fmt.Println(text)
}

func slashWrite(readLine func() (string, error), creds credentials, name string, project string, args map[string]any) {
	if project == "" {
		fmt.Println(red("choose a project first with /project <name|id>"))
		return
	}
	args["project"] = project
	if !confirmProposal(readLine, map[string]any{"name": name, "summary": name}) {
		return
	}
	text, err := callTool(creds, name, args)
	if err != nil {
		fmt.Println(red(err.Error()))
		return
	}
	fmt.Println(text)
}

func handleSlash(line string, readLine func() (string, error), creds credentials, project *string, cfg *cliConfig) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		slashMenu()
		return false
	}
	command := strings.ToLower(parts[0])
	args := parts[1:]
	switch command {
	case "/", "/help":
		slashMenu()
	case "/quit", "/exit":
		return true
	case "/project", "/use":
		slashProject(creds, args, project)
	case "/projects":
		slashProjects(creds)
	case "/keys":
		slashRead(creds, "key_inventory", *project, map[string]any{})
	case "/list-keys":
		slashRead(creds, "list_keys", *project, map[string]any{"page": 0})
	case "/stats":
		slashRead(creds, "project_stats", *project, map[string]any{})
	case "/setup":
		if len(args) != 1 {
			fmt.Println("usage: /setup <language>")
			break
		}
		slashRead(creds, "setup_wizard", *project, map[string]any{"language": args[0]})
	case "/add-key":
		count := 1
		duration := "Perm"
		if len(args) > 0 {
			if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
				count = parsed
			}
		}
		if len(args) > 1 {
			duration = strings.Join(args[1:], " ")
		}
		slashWrite(readLine, creds, "create_keys", *project, map[string]any{"count": count, "duration": duration})
	case "/delete-key", "/pause-key", "/unpause-key", "/reset-hwid":
		if len(args) != 1 {
			fmt.Println("usage: " + command + " <key>")
			break
		}
		name := map[string]string{"/delete-key": "delete_key", "/pause-key": "pause_key", "/unpause-key": "unpause_key", "/reset-hwid": "reset_hwid"}[command]
		slashWrite(readLine, creds, name, *project, map[string]any{"key": args[0]})
	case "/extend-key":
		if len(args) < 2 {
			fmt.Println("usage: /extend-key <key> <duration>")
			break
		}
		slashWrite(readLine, creds, "extend_key", *project, map[string]any{"key": args[0], "duration": strings.Join(args[1:], " ")})
	case "/key-note":
		if len(args) < 2 {
			fmt.Println("usage: /key-note <key> <note>")
			break
		}
		slashWrite(readLine, creds, "set_key_note", *project, map[string]any{"key": args[0], "note": strings.Join(args[1:], " ")})
	case "/status":
		_ = cmdStatus()
	case "/update":
		_ = cmdUpdate(nil)
	case "/effort":
		if len(args) == 1 && validEffort(args[0]) {
			cfg.Effort = args[0]
			_ = saveConfig(*cfg)
			fmt.Println("effort -> " + cfg.Effort)
		} else {
			fmt.Println("effort is " + cfg.Effort + " (low|medium|high)")
		}
	case "/perms":
		for _, tool := range agentTools {
			fmt.Printf("  %-10s %s\n", tool, permFor(*cfg, tool))
		}
	case "/call":
		if len(args) < 1 {
			fmt.Println("usage: /call <tool> [json]")
			break
		}
		callArgs := map[string]any{}
		if len(args) > 1 && json.Unmarshal([]byte(strings.Join(args[1:], " ")), &callArgs) != nil {
			fmt.Println(red("invalid JSON"))
			break
		}
		text, err := callTool(creds, args[0], callArgs)
		if err != nil {
			fmt.Println(red(err.Error()))
		} else {
			fmt.Println(text)
		}
	default:
		fmt.Println(dim("unknown command. Type / for options."))
	}
	return false
}

func printChatReply(payload map[string]any, announced *bool) []map[string]any {
	if !*announced {
		var scopes []string
		if list, ok := payload["scopes"].([]any); ok {
			for _, item := range list {
				if scope, ok := item.(string); ok {
					scopes = append(scopes, scope)
				}
			}
		}
		fmt.Println(dim("acting with scopes: " + strings.Join(scopes, " ")))
		fmt.Println()
		*announced = true
	}
	if reply, _ := payload["reply"].(string); strings.TrimSpace(reply) != "" {
		fmt.Println(renderMarkdown(strings.TrimSpace(reply)))
		fmt.Println()
	}
	if executed, ok := payload["executed"].([]any); ok {
		for _, item := range executed {
			entry, _ := item.(map[string]any)
			mark := green("✓ ok")
			if entry["ok"] != true {
				mark = red("✕ failed")
			}
			name, _ := entry["name"].(string)
			fmt.Printf("%s %s\n", bold("● "+name), mark)
			if entry["ok"] != true {
				if result, _ := entry["result"].(string); strings.TrimSpace(result) != "" {
					fmt.Println(red("  ⎿ " + strings.TrimSpace(result)))
				}
			}
		}
	}
	var proposals []map[string]any
	if list, ok := payload["proposals"].([]any); ok {
		for _, item := range list {
			if entry, ok := item.(map[string]any); ok && entry["name"] != "" {
				proposals = append(proposals, entry)
			}
		}
	}
	return proposals
}

func snippet(text string, limit int) string {
	flat := strings.Join(strings.Fields(text), " ")
	if len(flat) > limit {
		return flat[:limit] + "…"
	}
	if flat == "" {
		return "ok"
	}
	return flat
}

func confirmProposal(readLine func() (string, error), proposal map[string]any) bool {
	summary, _ := proposal["summary"].(string)
	if strings.TrimSpace(summary) == "" {
		summary, _ = proposal["name"].(string)
	}
	fmt.Printf("%s Approve %s? [y/N]: ", yellow("◆"), bold("'"+summary+"'"))
	answer, _ := readLine()
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

func askLocal(readLine func() (string, error), grants map[string]bool, cfg cliConfig, name string, args map[string]any) (bool, string) {
	if grants[name] {
		return true, ""
	}
	switch permFor(cfg, name) {
	case "allow":
		return true, ""
	case "deny":
		return false, "blocked by permissions (`authris perms set " + name + " ask` to allow)"
	}
	fmt.Printf("%s Allow %s? [y]es / [a]lways / [N]o: ", yellow("◆"), bold("'"+localCallSummary(name, args)+"'"))
	answer, _ := readLine()
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, ""
	case "a", "always":
		grants[name] = true
		return true, ""
	}
	return false, "declined by user"
}

func cmdAI(args []string) int {
	creds, err := loadCreds()
	if err != nil {
		fmt.Println("not logged in. Run `authris login --server <url>`")
		return 1
	}
	effortFlag := ""
	prompt := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--effort" && i+1 < len(args) {
			effortFlag = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
			continue
		}
		prompt = append(prompt, args[i])
	}
	cfg := loadConfig()
	if validEffort(effortFlag) {
		cfg.Effort = effortFlag
	}
	if err := lockAgentRoot(); err != nil {
		fmt.Println("error:", err)
		return 1
	}
	project := defaultProject(creds)
	reader := bufio.NewReader(os.Stdin)
	if !checkTerms(&cfg, reader) {
		return 1
	}
	var rl *readline.Instance
	readLine := func() (string, error) { return reader.ReadString('\n') }
	readlineConfig := &readline.Config{
		Prompt:      "❯ ",
		HistoryFile: filepath.Join(filepath.Dir(credPath()), "history"),
	}
	readlineConfig.SetListener(func(line []rune, pos int, key rune) ([]rune, int, bool) {
		if rl != nil {
			rl.SetPrompt(slashPrompt(string(line)))
		}
		return line, pos, true
	})
	if instance, openErr := readline.NewEx(readlineConfig); openErr == nil {
		rl = instance
		defer rl.Close()
		readLine = rl.Readline
	}
	printKeySystemHeader(creds, project)
	var history []map[string]any
	announced := false
	grants := map[string]bool{}
	remember := func(role string, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		history = append(history, map[string]any{"role": role, "content": content})
		if len(history) > 30 {
			history = history[len(history)-30:]
		}
	}
	note := func(format string, args ...any) {
		remember("user", "[agent note] "+fmt.Sprintf(format, args...))
	}
	round := func(message string) int {
		remember("user", message)
		payload := map[string]any{
			"project": project, "message": message, "history": history,
			"local_tools": true, "effort": cfg.Effort,
		}
		for step := 0; step < 25; step++ {
			label := "Thinking"
			if step > 0 {
				label = fmt.Sprintf("Working · step %d", step+1)
			}
			stop := startSpinner(label)
			resp, err := chatRequest(creds, payload)
			stop()
			if err != nil {
				fmt.Println(red("✕ " + err.Error()))
				return 1
			}
			if reply, _ := resp["reply"].(string); strings.TrimSpace(reply) != "" {
				remember("assistant", reply)
			}
			proposals := printChatReply(resp, &announced)
			if executedList, ok := resp["executed"].([]any); ok {
				for _, item := range executedList {
					entry, _ := item.(map[string]any)
					name, _ := entry["name"].(string)
					status := "ok"
					detail := ""
					if entry["ok"] != true {
						status = "failed"
						if result, _ := entry["result"].(string); strings.TrimSpace(result) != "" {
							detail = ": " + snippet(result, 200)
						}
					}
					note("server %s: %s%s", name, status, detail)
				}
			}
			var pending []any
			var results []any
			if list, ok := resp["local_calls"].([]any); ok {
				for _, item := range list {
					entry, ok := item.(map[string]any)
					if !ok {
						continue
					}
					id, _ := entry["id"].(string)
					name, _ := entry["name"].(string)
					if id == "" || !validTool(name) {
						continue
					}
					callArgs, _ := entry["arguments"].(map[string]any)
					if callArgs == nil {
						callArgs = map[string]any{}
					}
					pending = append(pending, map[string]any{"id": id, "name": name, "arguments": callArgs})
					allowed, reason := askLocal(readLine, grants, cfg, name, callArgs)
					summary := localCallSummary(name, callArgs)
					if !allowed {
						fmt.Printf("%s %s: %s\n", red("✕"), name, reason)
						note("user declined %s (%s)", name, reason)
						results = append(results, map[string]any{"id": id, "error": reason})
						continue
					}
					fmt.Printf("%s\n", bold("● "+summary))
					output, err := runLocalTool(name, callArgs)
					if err != nil {
						fmt.Printf("  %s %s\n", red("⎿"), err.Error())
						note("%s failed: %s", summary, snippet(err.Error(), 200))
						results = append(results, map[string]any{"id": id, "error": err.Error()})
						continue
					}
					note("%s done: %s", summary, snippet(output, 200))
					fmt.Printf("  %s\n", green("⎿ done"))
					results = append(results, map[string]any{"id": id, "output": output})
				}
			}
			var decided []map[string]any
			for _, proposal := range proposals {
				approved := confirmProposal(readLine, proposal)
				name, _ := proposal["name"].(string)
				if approved {
					note("user approved %s", name)
				} else {
					note("user declined %s", name)
				}
				decided = append(decided, map[string]any{
					"name": proposal["name"], "arguments": proposal["arguments"],
					"approved": approved,
				})
			}
			if len(pending) == 0 && len(decided) == 0 {
				return 0
			}
			payload = map[string]any{
				"project": project, "message": "", "history": history,
				"local_tools": true, "effort": cfg.Effort,
				"pending_calls": pending, "tool_results": results,
			}
			if len(decided) > 0 {
				payload["approvals"] = decided
			}
		}
		fmt.Println("stopped after 25 steps")
		return 0
	}
	if len(prompt) > 0 {
		return round(strings.Join(prompt, " "))
	}
	cwd, _ := os.Getwd()
	fmt.Println()
	fmt.Printf("  %s %s\n", bold("✻ authris key installer"), dim("v"+cliVersion))
	fmt.Printf("  %s\n", dim(cwd))
	fmt.Printf("  %s\n", dim("effort "+cfg.Effort+" · /help for commands · empty line quits"))
	fmt.Println()
	for {
		if rl == nil {
			fmt.Print(bold(cyan("❯") + " "))
		}
		line, err := readLine()
		if err != nil {
			fmt.Println()
			return 0
		}
		line = strings.TrimSpace(line)
		if rl != nil {
			rl.SetPrompt("❯ ")
		}
		if line == "" {
			return 0
		}
		if strings.HasPrefix(line, "/") {
			if handleSlash(line, readLine, creds, &project, &cfg) {
				return 0
			}
			continue
		}
		round(line)
	}
}

func usage() {
	fmt.Println("authris - Authris key-system installer")
	fmt.Println()
	fmt.Println("  authris                        start the AI chat")
	fmt.Println("  authris ai [prompt]            chat with the built-in assistant")
	fmt.Println("  authris login --server <url>   save a personal token")
	fmt.Println("  authris logout                 forget saved credentials")
	fmt.Println("  authris status                 show server, user, scopes")
	fmt.Println("  authris keys [project]         key inventory counts")
	fmt.Println("  authris call <tool> [json]     call any MCP tool")
	fmt.Println("  authris mcp                    stdio MCP bridge for local agents")
	fmt.Println("  authris perms                  show or change agent permissions")
	fmt.Println("  authris update                 update to the latest release")
	fmt.Println("  authris version                show the CLI version")
}

func main() {
	if len(os.Args) < 2 {
		cfg := loadConfig()
		maybeNotifyUpdate(&cfg)
		os.Exit(cmdAI(nil))
		return
	}
	if os.Args[1] != "update" && os.Args[1] != "version" && os.Args[1] != "help" && os.Args[1] != "-h" && os.Args[1] != "--help" {
		cfg := loadConfig()
		maybeNotifyUpdate(&cfg)
	}
	code := 0
	switch os.Args[1] {
	case "login":
		code = cmdLogin(os.Args[2:])
	case "logout":
		if err := os.Remove(credPath()); err != nil {
			fmt.Println("not logged in")
			break
		}
		fmt.Println("logged out")
	case "status":
		code = cmdStatus()
	case "keys":
		code = cmdKeys(os.Args[2:])
	case "call":
		code = cmdCall(os.Args[2:])
	case "mcp":
		code = cmdMCP()
	case "ai":
		code = cmdAI(os.Args[2:])
	case "perms":
		code = cmdPerms(os.Args[2:])
	case "update":
		code = cmdUpdate(os.Args[2:])
	case "version":
		fmt.Println("authris " + cliVersion)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		code = 1
	}
	os.Exit(code)
}
