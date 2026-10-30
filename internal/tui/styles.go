package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The palette is OpenCode's default theme.
var (
	bgColor = lipgloss.Color("#0a0a0a")
	panelBg = lipgloss.Color("#141414")
	amber   = lipgloss.Color("#fab283") // primary
	red     = lipgloss.Color("#e06c75")
	orange  = lipgloss.Color("#f5a742")
	yellow  = lipgloss.Color("#e5c07b")
	blue    = lipgloss.Color("#5c9cf5")
	purple  = lipgloss.Color("#9d7cd8")
	green   = lipgloss.Color("#7fd88f")
	gray    = lipgloss.Color("#808080")
	light   = lipgloss.Color("#eeeeee")
	dark    = lipgloss.Color("#434343")

	sAccent = lipgloss.NewStyle().Foreground(amber)
	sTitle  = lipgloss.NewStyle().Foreground(light).Bold(true)
	sTab    = lipgloss.NewStyle().Foreground(bgColor).Background(amber).Padding(0, 1)
	sTabOff = lipgloss.NewStyle().Foreground(gray).Padding(0, 1)
	sSel    = lipgloss.NewStyle().Foreground(bgColor).Background(amber)
	sDim    = lipgloss.NewStyle().Foreground(gray)
	sBold   = lipgloss.NewStyle().Foreground(light).Bold(true)
	sOK     = lipgloss.NewStyle().Foreground(green)
	sWarn   = lipgloss.NewStyle().Foreground(orange)
	sBad    = lipgloss.NewStyle().Foreground(red)
	sGroup  = lipgloss.NewStyle().Foreground(purple).Bold(true)
	sLogo   = lipgloss.NewStyle().Foreground(light).Bold(true)
)

func severityStyle(sev string) lipgloss.Style {
	switch sev {
	case "CRITICAL":
		return lipgloss.NewStyle().Foreground(red).Bold(true)
	case "HIGH":
		return lipgloss.NewStyle().Foreground(orange)
	case "MEDIUM":
		return lipgloss.NewStyle().Foreground(yellow)
	case "LOW":
		return lipgloss.NewStyle().Foreground(blue)
	}
	return sDim
}

func severityBadge(sev string) string {
	label := map[string]string{"CRITICAL": "CRIT", "HIGH": "HIGH", "MEDIUM": "MED", "LOW": "LOW"}[sev]
	if label == "" {
		label = "UNK"
	}
	return severityStyle(sev).Render(pad(label, 4))
}

func rankStyle(rank int) lipgloss.Style {
	return severityStyle([]string{"UNKNOWN", "LOW", "MEDIUM", "HIGH", "CRITICAL"}[min(max(rank, 0), 4)])
}

// paint fills a line to width w on background c, with the theme's text colour
// for unstyled text. Inner styles end in a full reset, which would drop both
// mid-line, so they're restored after each.
func paint(s string, c lipgloss.Color, w int) string {
	line := pad(s, w)
	seq, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(light).Background(c).Render("x"), "x")
	if seq == "" {
		return line // no colour support
	}
	return seq + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+seq) + "\x1b[0m"
}

// highlight turns the row carrying the cursor marker into a full-width
// selection bar. Without colour the marker stays, as the only cue.
func highlight(line string, w int) string {
	rest, ok := strings.CutPrefix(ansi.Strip(line), "▸ ")
	if !ok || sSel.Render("x") == "x" {
		return line
	}
	return sSel.Render(pad("  "+rest, w))
}

// panel is a window: a filled block with a bar down its left edge, lit when
// focused. It takes w by h, and its last row and column stay empty as a gutter.
func panel(lines []string, w, h int, focused bool) string {
	c := dark
	if focused {
		c = amber
	}
	bar := lipgloss.NewStyle().Foreground(c).Render("┃")
	inner := w - 3
	row := func(s string) string { return paint(bar+" "+highlight(s, inner), panelBg, w-1) + " " }

	out := make([]string, 0, h)
	out = append(out, row(""))
	for i := range max(0, h-2) {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, row(l))
	}
	return strings.Join(append(out, strings.Repeat(" ", w)), "\n")
}

// pad truncates or right-pads s (which may contain ANSI codes) to width w.
func pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func clip(s string, w int) string { return ansi.Truncate(s, w, "…") }
