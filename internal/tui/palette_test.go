package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func ctrlP(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	return next.(Model)
}

func typed(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	return m
}

func TestPaletteRuns(t *testing.T) {
	m := ctrlP(t, shellModel())
	if !m.palOpen || !strings.Contains(m.View(), "Commands") {
		t.Fatalf("palette closed:\n%s", m.View())
	}

	// Keys go to the search box, not the screen behind it.
	got := typed(t, m, "dep")
	if got.screen != screenHome || got.palInput.Value() != "dep" {
		t.Fatalf("screen %d, query %q", got.screen, got.palInput.Value())
	}
	got = press(t, got, "enter")
	if got.palOpen || got.screen != screenDeploys {
		t.Errorf("enter: open %v screen %d", got.palOpen, got.screen)
	}

	if got := press(t, m, "esc"); got.palOpen || got.screen != screenHome {
		t.Errorf("esc: open %v screen %d", got.palOpen, got.screen)
	}
	if got := ctrlP(t, m); got.palOpen {
		t.Error("ctrl+p should close it again")
	}
}

func TestPaletteOpensAProject(t *testing.T) {
	m := typed(t, ctrlP(t, shellModel()), "b")
	var at int
	for i, it := range m.palMatches() {
		if it.group == "Projects" {
			at = i
		}
	}
	m.palCur = at
	got := press(t, m, "enter")
	if got.screen != screenProject || got.scope != "/code/b" {
		t.Errorf("screen %d scope %q", got.screen, got.scope)
	}
}

func TestPaletteNoMatchIsSafe(t *testing.T) {
	m := typed(t, ctrlP(t, shellModel()), "zzzz")
	if !strings.Contains(m.View(), "no matches") {
		t.Errorf("view:\n%s", m.View())
	}
	if got := press(t, m, "down", "enter"); got.palOpen || got.screen != screenHome {
		t.Errorf("enter on nothing: open %v screen %d", got.palOpen, got.screen)
	}
}

func TestPaletteDoesNotOpenInAFlow(t *testing.T) {
	m := shellModel()
	m.screen, m.pickBack = screenPicker, screenDeploys
	if ctrlP(t, m).palOpen {
		t.Error("opened over the link picker")
	}
}

func TestPaletteFitsSmallTerminal(t *testing.T) {
	for _, sz := range [][2]int{{60, 14}, {80, 24}, {120, 30}} {
		m := shellModel()
		m.w, m.h = sz[0], sz[1]
		m = ctrlP(t, m)
		lines := strings.Split(m.View(), "\n")
		if len(lines) > m.h {
			t.Errorf("%dx%d: %d lines", m.w, m.h, len(lines))
		}
		for _, l := range lines {
			if lipgloss.Width(l) > m.w {
				t.Errorf("%dx%d: %d wide", m.w, m.h, lipgloss.Width(l))
			}
		}
	}
}

func TestBackgroundSurvivesInnerStyles(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := busyModel()
	for _, l := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(l) != m.w {
			t.Errorf("line is %d wide, want full width: %q", lipgloss.Width(l), l)
		}
		if !strings.Contains(l[:min(40, len(l))], "48;2;10;10;10m") {
			t.Errorf("line lost the background: %q", l)
		}
		if strings.Contains(strings.TrimSuffix(l, "\x1b[0m"), "\x1b[0m ") {
			t.Errorf("a reset left the background off: %q", l)
		}
	}
	got := paint(sBad.Render("x")+" y", panelBg, 10)
	if !strings.Contains(got, ";48;2;20;20;20m y") {
		t.Errorf("paint: %q", got)
	}
}

func TestSelectedRowIsABar(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	got := highlight(sAccent.Render("▸ ")+sBad.Render("devdoq"), 20)
	if !strings.Contains(got, "48;2;250;178;131") || !strings.Contains(got, "devdoq") || strings.Contains(got, "▸") {
		t.Errorf("selected: %q", got)
	}
	if lipgloss.Width(got) != 20 {
		t.Errorf("width %d", lipgloss.Width(got))
	}
	if got := highlight("  plain", 20); got != "  plain" {
		t.Errorf("unselected changed: %q", got)
	}
}
