package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const cliVersion = "4.2.1"
const cliRepo = "V12KLT/authris-cli"

func parseTagVersion(tag string) []int {
	trimmed := strings.TrimSpace(tag)
	trimmed = strings.TrimPrefix(trimmed, "cli-v")
	trimmed = strings.TrimPrefix(trimmed, "v")
	var out []int
	for _, part := range strings.Split(trimmed, ".") {
		digits := ""
		for _, rune := range part {
			if rune < '0' || rune > '9' {
				break
			}
			digits += string(rune)
		}
		if digits == "" {
			return nil
		}
		value, err := strconv.Atoi(digits)
		if err != nil {
			return nil
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func newerThan(tag string, current string) bool {
	remote := parseTagVersion(tag)
	local := parseTagVersion(current)
	if remote == nil || local == nil {
		return false
	}
	for i := 0; i < len(remote) || i < len(local); i++ {
		remotePart, localPart := 0, 0
		if i < len(remote) {
			remotePart = remote[i]
		}
		if i < len(local) {
			localPart = local[i]
		}
		if remotePart != localPart {
			return remotePart > localPart
		}
	}
	return false
}

func updateAssetName() (string, bool) {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return "", false
	}
	switch runtime.GOOS {
	case "windows":
		return "authris-windows-" + arch + ".exe", true
	case "linux":
		return "authris-linux-" + arch, true
	case "darwin":
		return "authris-darwin-" + arch, true
	}
	return "", false
}

func fetchLatestTag() (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + cliRepo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var parsed struct {
		Tag string `json:"tag_name"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Tag == "" {
		return "", fmt.Errorf("bad response")
	}
	return parsed.Tag, nil
}

func maybeNotifyUpdate(cfg *cliConfig) {
	if !cfg.AutoUpdate {
		return
	}
	now := time.Now().Unix()
	if cfg.UpdateChecked > 0 && now-cfg.UpdateChecked < 86400 {
		if cfg.UpdateAvailable != "" && newerThan(cfg.UpdateAvailable, cliVersion) {
			fmt.Printf("update %s available (you have %s). Run `authris update`.\n", cfg.UpdateAvailable, cliVersion)
		}
		return
	}
	tag, err := fetchLatestTag()
	cfg.UpdateChecked = now
	if err != nil {
		_ = saveConfig(*cfg)
		return
	}
	if newerThan(tag, cliVersion) {
		cfg.UpdateAvailable = tag
		fmt.Printf("update %s available (you have %s). Run `authris update`.\n", tag, cliVersion)
	} else {
		cfg.UpdateAvailable = ""
	}
	_ = saveConfig(*cfg)
}

func downloadAsset(tag string, asset string) (string, error) {
	url := "https://github.com/" + cliRepo + "/releases/download/" + tag + "/" + asset
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed (%d)", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "authris-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	written, err := io.Copy(tmp, io.LimitReader(resp.Body, 64<<20))
	if err != nil || written == 0 {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("download failed")
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func replaceSelf(fresh string) error {
	current, err := os.Executable()
	if err != nil {
		return err
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return err
	}
	backup := current + ".old"
	os.Remove(backup)
	if err := os.Rename(current, backup); err != nil {
		return err
	}
	if err := os.Rename(fresh, current); err != nil {
		os.Rename(backup, current)
		return err
	}
	os.Remove(backup)
	return nil
}

func cmdUpdate(args []string) int {
	force := len(args) > 0 && args[0] == "--force"
	asset, ok := updateAssetName()
	if !ok {
		fmt.Println("unsupported platform for auto-update")
		return 1
	}
	tag, err := fetchLatestTag()
	if err != nil {
		fmt.Println("could not check for updates:", err)
		return 1
	}
	if !force && !newerThan(tag, cliVersion) {
		fmt.Printf("already on latest (%s)\n", cliVersion)
		return 0
	}
	fmt.Printf("downloading %s...\n", tag)
	fresh, err := downloadAsset(tag, asset)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	if err := replaceSelf(fresh); err != nil {
		os.Remove(fresh)
		fmt.Println("could not install update:", err)
		return 1
	}
	cfg := loadConfig()
	cfg.UpdateAvailable = ""
	cfg.UpdateChecked = time.Now().Unix()
	_ = saveConfig(cfg)
	fmt.Printf("updated to %s. Re-run your command.\n", tag)
	return 0
}
