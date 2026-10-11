package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	ansiReset     = "\x1b[0m"
	ansiBold      = "\x1b[1m"
	ansiDim       = "\x1b[2m"
	ansiItalic    = "\x1b[3m"
	ansiUnderline = "\x1b[4m"
	ansiReverse   = "\x1b[7m"
	ansiCyan      = "\x1b[36m"
	ansiGreen     = "\x1b[32m"
	ansiYellow    = "\x1b[33m"
	ansiRed       = "\x1b[31m"
)

var colorsOn = initColors()

// selectionHighlight is the ANSI SGR prefix used to highlight the active row in
// slash-command and project pickers. Windows' legacy console ANSI parser (used by
// the vendored readline) only understands the basic 30-37/40-47 colors, so we fall
// back to a plain black-on-white highlight there instead of the 256-color variant.
var selectionHighlight = initSelectionHighlight()

func initSelectionHighlight() string {
	if runtime.GOOS == "windows" {
		return "\x1b[30;47m"
	}
	return "\x1b[48;5;236m"
}

func initColors() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if force := strings.TrimSpace(os.Getenv("FORCE_COLOR")); force != "" && force != "0" && force != "false" {
		if runtime.GOOS == "windows" {
			enableVirtualTerminal()
		}
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	if runtime.GOOS == "windows" && !enableVirtualTerminal() {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return true
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func paint(code string, text string) string {
	if !colorsOn || text == "" {
		return text
	}
	return code + text + ansiReset
}

func bold(text string) string {
	return paint(ansiBold, text)
}

func dim(text string) string {
	return paint(ansiDim, text)
}

func green(text string) string {
	return paint(ansiGreen, text)
}

func yellow(text string) string {
	return paint(ansiYellow, text)
}

func red(text string) string {
	return paint(ansiRed, text)
}

func cyan(text string) string {
	return paint(ansiCyan, text)
}

func codeSpan(text string) string {
	return paint(ansiReverse, text)
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func renderInline(text string) string {
	var out strings.Builder
	i := 0
	for i < len(text) {
		if text[i] == '`' {
			if end := strings.IndexByte(text[i+1:], '`'); end >= 0 {
				out.WriteString(codeSpan(text[i+1 : i+1+end]))
				i += end + 2
				continue
			}
		}
		if strings.HasPrefix(text[i:], "**") {
			if end := strings.Index(text[i+2:], "**"); end > 0 {
				out.WriteString(bold(text[i+2 : i+2+end]))
				i += end + 4
				continue
			}
		}
		if strings.HasPrefix(text[i:], "~~") {
			if end := strings.Index(text[i+2:], "~~"); end > 0 {
				out.WriteString(dim(text[i+2 : i+2+end]))
				i += end + 4
				continue
			}
		}
		if (text[i] == '*' || text[i] == '_') && (i == 0 || !isWordByte(text[i-1])) {
			if end := strings.IndexByte(text[i+1:], text[i]); end > 0 {
				inner := text[i+1 : i+1+end]
				after := i + 1 + end + 1
				if !strings.Contains(inner, " ") || after >= len(text) || !isWordByte(text[after]) {
					if !strings.ContainsAny(inner, "*_`") {
						out.WriteString(paint(ansiItalic, inner))
						i = after
						continue
					}
				}
			}
		}
		if text[i] == '[' {
			rest := text[i:]
			endText := strings.IndexByte(rest, ']')
			if endText > 1 && endText+1 < len(rest) && rest[endText+1] == '(' {
				endURL := strings.IndexByte(rest[endText+2:], ')')
				if endURL > 0 {
					label := rest[1:endText]
					url := rest[endText+2 : endText+2+endURL]
					out.WriteString(paint(ansiUnderline, label))
					out.WriteString(dim(" (" + url + ")"))
					i += endText + 2 + endURL + 1
					continue
				}
			}
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String()
}

func listItem(text string) (string, bool) {
	for _, marker := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(text, marker); ok {
			return rest, true
		}
	}
	i := 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(text) && (text[i] == '.' || text[i] == ')') && text[i+1] == ' ' {
		return text[i+2:], true
	}
	return "", false
}

func renderMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fence, ok := strings.CutPrefix(trimmed, "```"); ok {
			if !inFence {
				inFence = true
				label := strings.TrimSpace(fence)
				if label == "" {
					label = "code"
				}
				out = append(out, dim("┌─ "+label))
			} else {
				inFence = false
				out = append(out, dim("└─"))
			}
			continue
		}
		if inFence {
			out = append(out, dim("│ ")+line)
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			if level <= 6 && level < len(trimmed) && trimmed[level] == ' ' {
				out = append(out, paint(ansiBold+ansiCyan, strings.TrimSpace(trimmed[level:])))
				continue
			}
		}
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			out = append(out, dim(strings.Repeat("─", 32)))
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, ">"); ok {
			out = append(out, dim("│ ")+renderInline(strings.TrimSpace(rest)))
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if rest, ok := listItem(trimmed); ok {
			out = append(out, indent+cyan("•")+" "+renderInline(rest))
			continue
		}
		out = append(out, renderInline(line))
	}
	return strings.Join(out, "\n")
}

// takeLines splits text into at most limit lines, reporting whether it was
// truncated and how many lines there were in total.
func takeLines(text string, limit int) ([]string, bool, int) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, false, 0
	}
	total := len(lines)
	if total > limit {
		return lines[:limit], true, total
	}
	return lines, false, total
}

// highlightCode gives code a bit of colour: whole-line comments dim, and
// quoted strings cyan. It is intentionally lightweight.
func highlightCode(line string) string {
	if !colorsOn {
		return line
	}
	trimmed := strings.TrimSpace(line)
	for _, marker := range []string{"#", "//", "--", "/*", "*", ";"} {
		if strings.HasPrefix(trimmed, marker) {
			return dim(line)
		}
	}
	var b strings.Builder
	i := 0
	for i < len(line) {
		c := line[i]
		if c == '"' || c == '\'' || c == '`' {
			quote := c
			j := i + 1
			for j < len(line) {
				if line[j] == '\\' {
					j += 2
					continue
				}
				if line[j] == quote {
					j++
					break
				}
				j++
			}
			if j > len(line) {
				j = len(line)
			}
			b.WriteString(cyan(line[i:j]))
			i = j
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// codeBlock renders lines as a bordered, code-like block with a gutter.
func codeBlock(label string, lines []string, truncated bool, total int) string {
	if label == "" {
		label = "output"
	}
	var b strings.Builder
	b.WriteString("  " + dim("┌─ "+label))
	for _, line := range lines {
		b.WriteString("\n  " + dim("│ ") + line)
	}
	if truncated && total > len(lines) {
		b.WriteString("\n  " + dim("│ ") + dim(fmt.Sprintf("… %d more lines", total-len(lines))))
	}
	b.WriteString("\n  " + dim("└─"))
	return b.String()
}

func stripReadHeader(output string) string {
	if strings.HasPrefix(output, "Read file ") {
		if idx := strings.IndexByte(output, '\n'); idx >= 0 {
			return output[idx+1:]
		}
	}
	return output
}

// toolPreview renders a short, code-styled excerpt of what a local tool did:
// the lines it read, or the +/- lines an edit or write changed.
func toolPreview(name string, args map[string]any, output string) string {
	switch name {
	case "fs_read":
		lines, truncated, total := takeLines(stripReadHeader(output), 16)
		styled := make([]string, len(lines))
		for i, line := range lines {
			styled[i] = highlightCode(line)
		}
		return codeBlock(toolPath(args), styled, truncated, total)
	case "fs_list":
		lines, truncated, total := takeLines(output, 18)
		return codeBlock(toolPath(args)+"/", lines, truncated, total)
	case "fs_write":
		raw, truncated, total := takeLines(toolArg(args, "content", "text", "data", "body"), 16)
		lines := make([]string, len(raw))
		for i, line := range raw {
			lines[i] = green("+") + " " + highlightCode(line)
		}
		return codeBlock(toolPath(args), lines, truncated, total)
	case "fs_edit":
		oldText := toolArg(args, "old_text", "old", "old_string", "oldString", "find", "search")
		newText := toolArg(args, "new_text", "new", "new_string", "newString", "replace", "replacement")
		oldLines, _, _ := takeLines(oldText, 40)
		newLines, _, _ := takeLines(newText, 40)
		var lines []string
		for _, line := range oldLines {
			lines = append(lines, red("-")+" "+cleanDiffLine(line))
		}
		for _, line := range newLines {
			lines = append(lines, green("+")+" "+cleanDiffLine(line))
		}
		truncated := false
		total := len(lines)
		if total > 20 {
			lines = lines[:20]
			truncated = true
		}
		return codeBlock(toolPath(args), lines, truncated, total)
	case "exec":
		lines, truncated, total := takeLines(output, 12)
		styled := make([]string, len(lines))
		for i, line := range lines {
			styled[i] = highlightCode(line)
		}
		return codeBlock(strings.Join([]string{"$", toolArg(args, "command", "cmd", "script")}, " "), styled, truncated, total)
	}
	return ""
}

// cleanDiffLine drops a trailing carriage return so diff markers line up.
func cleanDiffLine(line string) string {
	return strings.TrimRight(line, "\r")
}

func startSpinner(label string) func() {
	noop := func() {}
	if !colorsOn {
		return noop
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		start := time.Now()
		i := 0
		for {
			select {
			case <-done:
				fmt.Print("\r\x1b[2K")
				return
			case <-ticker.C:
				fmt.Printf("\r%s %s %s", cyan(frames[i%len(frames)]), dim(label), dim(fmt.Sprintf("(%ds)", int(time.Since(start).Seconds()))))
				i++
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
}
