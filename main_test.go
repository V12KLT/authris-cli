package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewerThan(t *testing.T) {
	cases := []struct {
		tag     string
		current string
		want    bool
	}{
		{"cli-v4.1.0", "4.0.0", true},
		{"cli-v4.0.0", "4.0.0", false},
		{"cli-v4.0.1", "4.0.0", true},
		{"cli-v3.9.9", "4.0.0", false},
		{"cli-v4.10.0", "4.9.0", true},
		{"v4.1.0", "4.0.0", true},
		{"bogus", "4.0.0", false},
		{"cli-v4.1.0", "bogus", false},
	}
	for _, tc := range cases {
		if got := newerThan(tc.tag, tc.current); got != tc.want {
			t.Errorf("newerThan(%q, %q) = %v, want %v", tc.tag, tc.current, got, tc.want)
		}
	}
}

func TestInsideRoot(t *testing.T) {
	root := t.TempDir()
	agentRoot = root
	defer func() { agentRoot = "" }()
	if _, err := insideRoot("sub/file.txt"); err != nil {
		t.Fatalf("relative path = %v", err)
	}
	if _, err := insideRoot("../escape.txt"); err == nil {
		t.Fatal("expected escape to fail")
	}
	if _, err := insideRoot(filepath.Join(root, "ok.txt")); err != nil {
		t.Fatalf("absolute inside = %v", err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if _, err := insideRoot(outside); err == nil {
		t.Fatal("expected outside absolute to fail")
	}
	if got, err := insideRoot(""); err != nil || got != root {
		t.Fatalf("empty path = %q %v", got, err)
	}
}

func TestFSEditUnique(t *testing.T) {
	root := t.TempDir()
	agentRoot = root
	defer func() { agentRoot = "" }()
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("one two one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runLocalTool("fs_edit", map[string]any{"path": "a.txt", "old_text": "one", "new_text": "1"}); err == nil {
		t.Fatal("expected non-unique edit to fail")
	}
	if _, err := runLocalTool("fs_edit", map[string]any{"path": "a.txt", "old_text": "two", "new_text": "2"}); err != nil {
		t.Fatalf("edit = %v", err)
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "one 2 one" {
		t.Fatalf("content = %q", raw)
	}
	if _, err := runLocalTool("fs_write", map[string]any{"path": "../evil.txt", "content": "x"}); err == nil {
		t.Fatal("expected outside write to fail")
	}
}

func TestPermFor(t *testing.T) {
	cfg := defaultConfig()
	if permFor(cfg, "fs_read") != "allow" {
		t.Fatalf("fs_read = %s", permFor(cfg, "fs_read"))
	}
	if permFor(cfg, "exec") != "ask" {
		t.Fatalf("exec = %s", permFor(cfg, "exec"))
	}
	cfg.Permissions["exec"] = "deny"
	if permFor(cfg, "exec") != "deny" {
		t.Fatalf("exec = %s", permFor(cfg, "exec"))
	}
	cfg.Permissions["exec"] = "bogus"
	if permFor(cfg, "exec") != "ask" {
		t.Fatalf("invalid level should fall back, got %s", permFor(cfg, "exec"))
	}
}

func TestValidEffort(t *testing.T) {
	for _, level := range []string{"low", "medium", "high"} {
		if !validEffort(level) {
			t.Errorf("%s should be valid", level)
		}
	}
	if validEffort("ultra") || validEffort("") {
		t.Error("unexpected levels accepted")
	}
}
