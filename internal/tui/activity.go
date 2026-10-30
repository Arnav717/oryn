package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/browser"
	"github.com/Arnav717/oryn/internal/store"
)

// activityFilters are what f cycles through. nil kinds means everything.
var activityFilters = []struct {
	name  string
	kinds []string
}{
	{"all", nil},
	{"deploys", []string{"deploy"}},
	{"github", []string{"pr", "issue", "ci", "review"}},
	{"health", []string{"vuln", "release", "status"}},
}

// activityScope is the project the timeline is limited to, "" for all.
func (m Model) activityScope() string {
	if m.screen == screenProject {
		return m.scope
	}
	return ""
}

// shownEvent applies the filter and, inside a project, keeps what belongs to
// it. Upstream status isn't any one project's, so it stays on the top tab.
func (m Model) shownEvent(e store.Event) bool {
	if e.Kind == "repo" {
		return false // repo baselines are bookkeeping
	}
	if k := activityFilters[m.chFilter].kinds; k != nil && !slices.Contains(k, e.Kind) {
		return false
	}
	project := m.activityScope()
	if project == "" {
		return true
	}
	if e.Entity == project {
		return true
	}
	if ref, ok := m.repoRefFor(project); ok && e.Entity == ref.String() {
		return true
	}
	_, ok := m.pkgs[e.Entity]
	return ok
}

// activity builds the timeline: what's new since your last visit, most severe
// first, then everything earlier, newest first. It returns the body lines, the
// selectable rows, and the line each row sits on.
func (m Model) activity() (lines []string, rows []sinceRow, rowLine []int) {
	if m.snap == nil {
		return []string{sDim.Render("  loading…")}, nil, nil
	}
	build := func(evs []store.Event) []sinceRow {
		if !m.chExpand {
			return groupSince(evs)
		}
		out := make([]sinceRow, len(evs))
		for i, e := range evs {
			out[i] = sinceRow{e.Severity, eventLine(e), e.URL, []store.Event{e}}
		}
		return out
	}

	var fresh, earlier []store.Event
	seen := map[string]bool{}
	for _, e := range m.sinceEvents() {
		seen[e.Key] = true
		if m.shownEvent(e) {
			fresh = append(fresh, e)
		}
	}
	for _, e := range m.snap.Events {
		if !seen[e.Key] && m.shownEvent(e) {
			earlier = append(earlier, e)
		}
	}

	head := sTitle.Render("Activity") + sDim.Render("  "+activityFilters[m.chFilter].name)
	if !m.since.IsZero() {
		head += sDim.Render(fmt.Sprintf(" · %d new", len(fresh)))
	}
	lines = []string{head}
	block := func(title string, rs []sinceRow) {
		if len(rs) == 0 {
			return
		}
		lines = append(lines, "", sDim.Bold(true).Render(title))
		for _, r := range rs {
			marker := "  "
			if len(rows) == m.chCursor {
				marker = sAccent.Render("▸ ")
			}
			rowLine = append(rowLine, len(lines))
			rows = append(rows, r)
			lines = append(lines, marker+r.line)
		}
	}
	block("New since you last looked", build(fresh))
	block("Earlier", build(earlier))
	if len(rows) == 0 {
		lines = append(lines, "", sDim.Render("  Nothing here yet."))
	}
	return lines, rows, rowLine
}

func (m Model) activityBody() string {
	lines, _, _ := m.activity()
	return m.fullBox(lines[min(m.chOffset, len(lines)):])
}

func (m *Model) resetActivity() {
	m.chCursor, m.chOffset, m.chExpand, m.chFilter = 0, 0, false, 0
	m.chClamp()
}

func (m *Model) chClamp() {
	_, rows, rowLine := m.activity()
	m.chCursor = max(0, min(m.chCursor, len(rows)-1))
	if m.h == 0 || len(rowLine) == 0 {
		m.chOffset = 0
		return
	}
	vis := max(1, m.h-m.chrome()-2)
	line := rowLine[m.chCursor]
	// Keep the section title above the first row in view.
	if m.chCursor == 0 {
		line = 0
	}
	if line < m.chOffset {
		m.chOffset = line
	}
	if rowLine[m.chCursor] >= m.chOffset+vis {
		m.chOffset = rowLine[m.chCursor] - vis + 1
	}
	m.chOffset = max(0, m.chOffset)
}

func (m Model) handleActivityKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.screen == screenProject {
			m.setTab(tabOverview)
		} else {
			m.openSection(secHome)
		}
	case "]":
		m.switchTab(1)
	case "[":
		m.switchTab(-1)
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "j", "down":
		m.chCursor++
	case "k", "up":
		m.chCursor--
	case "pgdown", "ctrl+d":
		m.chCursor += max(1, m.h-m.chrome()-2)
	case "pgup", "ctrl+u":
		m.chCursor -= max(1, m.h-m.chrome()-2)
	case "g", "home":
		m.chCursor = 0
	case "G", "end":
		m.chCursor = 1 << 30
	case "e":
		m.chExpand = !m.chExpand
		m.chCursor, m.chOffset = 0, 0
	case "f":
		m.chFilter = (m.chFilter + 1) % len(activityFilters)
		m.chCursor, m.chOffset = 0, 0
	case "enter", "o":
		if _, rows, _ := m.activity(); m.chCursor >= 0 && m.chCursor < len(rows) {
			if url := rows[m.chCursor].url; url == "" {
				m.flash = "no link for this event"
			} else if err := browser.Open(url); err != nil {
				m.flash = err.Error()
			} else {
				m.flash = "opened in browser"
			}
		}
	}
	m.chClamp()
	return m, nil
}
