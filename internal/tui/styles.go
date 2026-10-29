package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	amber  = lipgloss.Color("#F5A623")
	red    = lipgloss.Color("#FF453A")
	orange = lipgloss.Color("#FF9F0A")
	yellow = lipgloss.Color("#FFD60A")
	blue   = lipgloss.Color("#64D2FF")
	green  = lipgloss.Color("#32D74B")
	gray   = lipgloss.Color("#8E8E93")
	dark   = lipgloss.Color("#3A3A3C")

	sAccent = lipgloss.NewStyle().Foreground(amber)
	sTitle  = lipgloss.NewStyle().Foreground(amber).Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(gray)
	sBold   = lipgloss.NewStyle().Bold(true)
	sOK     = lipgloss.NewStyle().Foreground(green)
	sWarn   = lipgloss.NewStyle().Foreground(yellow)
	sBad    = lipgloss.NewStyle().Foreground(red)
	sLogo   = lipgloss.NewStyle().Background(amber).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 1)
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

func paneBox(focused bool) lipgloss.Style {
	c := dark
	if focused {
		c = amber
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c)
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
