package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/source"
)

func ghModel(review, mine []source.PR) Model {
	m := testModel("", "/code/a")
	m.w, m.h = 120, 20
	m.snap = &app.Snapshot{GitHub: &source.GitHubData{Login: "me", Review: review, Mine: mine}}
	return m
}

func pr(n int, ci, review string) source.PR {
	return source.PR{Repo: "o/r", Number: n, Title: fmt.Sprintf("pr %d", n), CI: ci, Review: review, Updated: time.Now()}
}

func TestMinePRsNeedingAttentionFirst(t *testing.T) {
	m := ghModel(nil, []source.PR{
		pr(1, "SUCCESS", "APPROVED"),
		pr(2, "SUCCESS", "CHANGES_REQUESTED"),
		pr(3, "FAILURE", ""),
		pr(4, "", ""),
	})
	var got []int
	for _, p := range m.ghPRs() {
		got = append(got, p.Number)
	}
	want := []int{3, 2, 1, 4} // failing CI, changes requested, then original order
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if r, mine, failing := m.ghSummary(); r != 0 || mine != 4 || failing != 1 {
		t.Errorf("summary = %d %d %d", r, mine, failing)
	}
}

func TestGHScrollKeepsCursorVisible(t *testing.T) {
	var many []source.PR
	for i := 1; i <= 40; i++ {
		many = append(many, pr(i, "SUCCESS", ""))
	}
	m := ghModel(many[:5], many)
	vis := m.h - 4
	for i := 0; i < 45; i++ {
		m.ghCursor++
		m.ghClamp()
		_, line, top := m.ghLines()
		if line < m.ghOffset || line >= m.ghOffset+vis {
			t.Fatalf("step %d: cursor line %d outside window [%d,%d)", i, line, m.ghOffset, m.ghOffset+vis)
		}
		_ = top
	}
	if want := 5 + 40 - 1; m.ghCursor != want {
		t.Errorf("cursor = %d, want clamped to %d", m.ghCursor, want)
	}
	for i := 0; i < 60; i++ { // and back up to the top, revealing the first header
		m.ghCursor--
		m.ghClamp()
	}
	if m.ghCursor != 0 || m.ghOffset != 0 {
		t.Errorf("after scrolling up: cursor=%d offset=%d", m.ghCursor, m.ghOffset)
	}
}

func TestHumanAge(t *testing.T) {
	cases := map[time.Duration]string{
		10 * time.Second:    "now",
		5 * time.Minute:     "5m",
		3 * time.Hour:       "3h",
		72 * time.Hour:      "3d",
		90 * 24 * time.Hour: "3mo",
	}
	for d, want := range cases {
		if got := humanAge(time.Now().Add(-d)); got != want {
			t.Errorf("humanAge(-%v) = %q, want %q", d, got, want)
		}
	}
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
