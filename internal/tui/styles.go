package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	amber  = lipgloss.Color("#E8B15A")
	red    = lipgloss.Color("#F07178")
	orange = lipgloss.Color("#F2A36B")
	yellow = lipgloss.Color("#E6C778")
	blue   = lipgloss.Color("#8CC8E8")
	green  = lipgloss.Color("#7FCB98")
	gray   = lipgloss.Color("#7C7F88")
	light  = lipgloss.Color("#E4E4E7")

	sAccent = lipgloss.NewStyle().Foreground(amber)
	sTitle  = lipgloss.NewStyle().Foreground(light).Bold(true)
	sTab    = lipgloss.NewStyle().Foreground(amber).Bold(true).Underline(true)
	sDim    = lipgloss.NewStyle().Foreground(gray)
	sBold   = lipgloss.NewStyle().Bold(true)
	sOK     = lipgloss.NewStyle().Foreground(green)
	sWarn   = lipgloss.NewStyle().Foreground(yellow)
	sBad    = lipgloss.NewStyle().Foreground(red)
	sLogo   = lipgloss.NewStyle().Foreground(amber).Bold(true)
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

// box holds a body or window. The border is invisible: it keeps the margin,
// and whitespace does the separating.
func box() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.HiddenBorder())
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
