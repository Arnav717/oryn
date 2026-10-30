package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// oryn's palette: cool blue-black, with one teal accent for whatever is active.
var (
	bgColor    = lipgloss.Color("#0b0e14")
	panelBg    = lipgloss.Color("#10141c")
	selBg      = lipgloss.Color("#14303a")
	accent     = lipgloss.Color("#4fd1c5")
	red        = lipgloss.Color("#f2777a")
	orange     = lipgloss.Color("#f5b971")
	yellow     = lipgloss.Color("#ecd08a")
	blue       = lipgloss.Color("#6cb6ff")
	periwinkle = lipgloss.Color("#8aa4ff")
	green      = lipgloss.Color("#6ee7a8")
	gray       = lipgloss.Color("#7d8796")
	light      = lipgloss.Color("#e6e9ef")
	dark       = lipgloss.Color("#2a3140")

	sAccent = lipgloss.NewStyle().Foreground(accent)
	sTitle  = lipgloss.NewStyle().Foreground(light).Bold(true)
	sTab    = lipgloss.NewStyle().Foreground(accent).Bold(true).Underline(true).UnderlineSpaces(false).Padding(0, 1)
	sTabOff = lipgloss.NewStyle().Foreground(gray).Padding(0, 1)
	sSel    = lipgloss.NewStyle().Foreground(light).Background(selBg).Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(gray)
	sBold   = lipgloss.NewStyle().Foreground(light).Bold(true)
	sOK     = lipgloss.NewStyle().Foreground(green)
	sWarn   = lipgloss.NewStyle().Foreground(orange)
	sBad    = lipgloss.NewStyle().Foreground(red)
	sGroup  = lipgloss.NewStyle().Foreground(periwinkle).Bold(true)
	sLogo   = lipgloss.NewStyle().Foreground(accent).Bold(true)
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

// selBar draws a selected row: a tinted band with an accent edge.
func selBar(text string, w int) string {
	edge := lipgloss.NewStyle().Foreground(accent).Background(selBg).Render("▌")
	return edge + sSel.Render(pad(" "+text, w-1))
}

// highlight turns the row carrying the cursor marker into a selected row.
// Without colour the marker stays, as the only cue.
func highlight(line string, w int) string {
	rest, ok := strings.CutPrefix(ansi.Strip(line), "▸ ")
	if !ok || sSel.Render("x") == "x" {
		return line
	}
	return selBar(rest, w)
}

// panel is a window: a square frame with its title set into the top edge, lit
// when focused. It fills exactly w by h.
func panel(title string, lines []string, w, h int, focused bool) string {
	c, name := dark, sDim.Bold(true).Render(title)
	if focused {
		c, name = accent, sAccent.Bold(true).Render(title)
	}
	edge := lipgloss.NewStyle().Foreground(c)
	inner := w - 4

	out := make([]string, 0, h)
	rule := max(0, w-5-lipgloss.Width(title))
	out = append(out, paint(edge.Render("┌─ ")+name+edge.Render(" "+strings.Repeat("─", rule)+"┐"), panelBg, w))
	for i := range max(0, h-2) {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, paint(edge.Render("│")+" "+pad(highlight(l, inner), inner)+" "+edge.Render("│"), panelBg, w))
	}
	out = append(out, paint(edge.Render("└"+strings.Repeat("─", max(0, w-2))+"┘"), panelBg, w))
	return strings.Join(out, "\n")
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
