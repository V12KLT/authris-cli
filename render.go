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

func initColors() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
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
