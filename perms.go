package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var agentTools = []string{"fs_list", "fs_read", "fs_write", "fs_edit", "exec"}

var defaultPerms = map[string]string{
	"fs_list": "allow", "fs_read": "allow",
	"fs_write": "allow", "fs_edit": "allow", "exec": "allow",
}

const configRev = 1
const termsRev = 1

var agentTerms = "Authris AI agent terms\n\n" +
	"This assistant works directly in the folder where you run it: it can create, edit, and overwrite files there and run terminal commands. " +
	"It cannot reach outside that folder, but commands it runs can still change or delete your files. " +
	"Review what it does and keep backups of anything important. " +
	"By accepting, you agree that you are responsible for what the agent does in your project.\n\n" +
	"Type 'accept' to continue: "

type cliConfig struct {
	Permissions     map[string]string `json:"permissions"`
	Effort          string            `json:"effort"`
	AutoUpdate      bool              `json:"auto_update"`
	UpdateChecked   int64             `json:"update_checked"`
	UpdateAvailable string            `json:"update_available"`
	Rev             int               `json:"rev"`
	TermsRev        int               `json:"terms_rev"`
}

var configDirOverride = ""

func configPath() string {
	if configDirOverride != "" {
		return filepath.Join(configDirOverride, "authris", "config.json")
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "authris", "config.json")
}

func defaultConfig() cliConfig {
	perms := map[string]string{}
	for tool, level := range defaultPerms {
		perms[tool] = level
	}
	return cliConfig{Permissions: perms, Effort: "medium", AutoUpdate: true, Rev: configRev}
}

func loadConfig() cliConfig {
	cfg := defaultConfig()
	raw, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	var stored cliConfig
	if err := json.Unmarshal(raw, &stored); err != nil {
		return cfg
	}
	if stored.Permissions != nil {
		for tool, level := range stored.Permissions {
			if validTool(tool) && validPerm(level) {
				cfg.Permissions[tool] = level
			}
		}
	}
	if stored.Rev < configRev {
		for _, tool := range []string{"fs_write", "fs_edit", "exec"} {
			if cfg.Permissions[tool] == "ask" {
				cfg.Permissions[tool] = defaultPerms[tool]
			}
		}
		cfg.Rev = configRev
		_ = saveConfig(cfg)
	} else {
		cfg.Rev = stored.Rev
	}
	if validEffort(stored.Effort) {
		cfg.Effort = stored.Effort
	}
	cfg.AutoUpdate = stored.AutoUpdate
	cfg.UpdateChecked = stored.UpdateChecked
	cfg.UpdateAvailable = stored.UpdateAvailable
	cfg.TermsRev = stored.TermsRev
	return cfg
}

func saveConfig(cfg cliConfig) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func validTool(tool string) bool {
	for _, name := range agentTools {
		if name == tool {
			return true
		}
	}
	return false
}

func validPerm(level string) bool {
	return level == "allow" || level == "ask" || level == "deny"
}

func validEffort(level string) bool {
	return level == "low" || level == "medium" || level == "high"
}

func permFor(cfg cliConfig, tool string) string {
	if level, ok := cfg.Permissions[tool]; ok {
		if validPerm(level) {
			return level
		}
		return "ask"
	}
	if level, ok := defaultPerms[tool]; ok {
		return level
	}
	return "ask"
}

func checkTerms(cfg *cliConfig, reader *bufio.Reader) bool {
	if cfg.TermsRev >= termsRev {
		return true
	}
	fmt.Println()
	fmt.Print(agentTerms)
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "accept" {
		fmt.Println("terms declined. The agent cannot run until you accept.")
		return false
	}
	cfg.TermsRev = termsRev
	_ = saveConfig(*cfg)
	fmt.Println("terms accepted.")
	fmt.Println()
	return true
}

func cmdPerms(args []string) int {
	cfg := loadConfig()
	if len(args) == 0 {
		fmt.Printf("%-10s %s\n", "tool", "permission")
		for _, tool := range agentTools {
			fmt.Printf("%-10s %s\n", tool, permFor(cfg, tool))
		}
		fmt.Printf("%-10s %s\n", "effort", cfg.Effort)
		fmt.Printf("%-10s %v\n", "autoupdate", cfg.AutoUpdate)
		return 0
	}
	if len(args) == 3 && args[0] == "set" && validTool(args[1]) && validPerm(args[2]) {
		cfg.Permissions[args[1]] = args[2]
		if err := saveConfig(cfg); err != nil {
			fmt.Println("could not save config:", err)
			return 1
		}
		fmt.Printf("%s -> %s\n", args[1], args[2])
		return 0
	}
	if len(args) == 2 && args[0] == "effort" && validEffort(args[1]) {
		cfg.Effort = args[1]
		if err := saveConfig(cfg); err != nil {
			fmt.Println("could not save config:", err)
			return 1
		}
		fmt.Printf("effort -> %s\n", args[1])
		return 0
	}
	if len(args) == 2 && args[0] == "autoupdate" && (args[1] == "on" || args[1] == "off") {
		cfg.AutoUpdate = args[1] == "on"
		if err := saveConfig(cfg); err != nil {
			fmt.Println("could not save config:", err)
			return 1
		}
		fmt.Printf("autoupdate -> %s\n", args[1])
		return 0
	}
	fmt.Println("usage:")
	fmt.Println("  authris perms                     show permissions")
	fmt.Println("  authris perms set <tool> <allow|ask|deny>")
	fmt.Println("  authris perms effort <low|medium|high>")
	fmt.Println("  authris perms autoupdate <on|off>")
	fmt.Println("tools: " + strings.Join(agentTools, ", "))
	return 1
}
