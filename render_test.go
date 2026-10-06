package main

import (
	"strings"
	"testing"
)

func TestRenderInlinePlain(t *testing.T) {
	colorsOn = false
	defer func() { colorsOn = initColors() }()
	cases := []struct {
		in   string
		want string
	}{
		{"**bold**", "bold"},
		{"*italic*", "italic"},
		{"`code`", "code"},
		{"~~gone~~", "gone"},
		{"[docs](https://x.test)", "docs (https://x.test)"},
		{"some_var_name", "some_var_name"},
		{"2*3", "2*3"},
		{"a **b** c", "a b c"},
		{"unclosed **bold", "unclosed **bold"},
	}
	for _, tc := range cases {
		if got := renderInline(tc.in); got != tc.want {
			t.Errorf("renderInline(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderInlineColored(t *testing.T) {
	colorsOn = true
	defer func() { colorsOn = initColors() }()
	if got := renderInline("**b**"); !strings.Contains(got, ansiBold) {
		t.Errorf("expected bold code in %q", got)
	}
	if got := renderInline("`c`"); !strings.Contains(got, ansiReverse) {
		t.Errorf("expected reverse code in %q", got)
	}
}

func TestRenderMarkdownBlocks(t *testing.T) {
	colorsOn = false
	defer func() { colorsOn = initColors() }()
	in := "# Title\n\n- one\n- **two**\n\n```go\nx := 1\n```\n\n> note\n\n[docs](https://x.test)"
	got := renderMarkdown(in)
	for _, want := range []string{"Title", "• one", "• two", "┌─ go", "│ x := 1", "└─", "│ note", "docs (https://x.test)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	for _, banned := range []string{"# Title", "**", "```", "- one", "> note", "]("} {
		if strings.Contains(got, banned) {
			t.Errorf("did not expect %q in:\n%s", banned, got)
		}
	}
}
