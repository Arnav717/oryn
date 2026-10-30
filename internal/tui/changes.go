package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/browser"
	"github.com/Arnav717/oryn/internal/store"
)

// changeRows is what the changes screen lists: grouped by package, or every
// event when expanded.
func (m Model) changeRows() []sinceRow {
	evs := m.sinceEvents()
	if m.chExpand {
		rows := make([]sinceRow, len(evs))
		for i, e := range evs {
			rows[i] = sinceRow{e.Severity, eventLine(e), e.URL, []store.Event{e}}
		}
		return rows
	}
	return groupSince(evs)
}

func (m *Model) chClamp() {
	n := len(m.changeRows())
	m.chCursor = max(0, min(m.chCursor, n-1))
	if m.h == 0 {
		return
	}
	vis := max(1, m.h-m.chrome()-2)
	line := m.chCursor + 2 // two header lines precede the rows
	if line < m.chOffset {
		m.chOffset = line
	}
	if line >= m.chOffset+vis {
		m.chOffset = line - vis + 1
	}
	m.chOffset = max(0, m.chOffset)
}

func (m Model) handleChangesKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "h", "c", "backspace", "left":
		m.screen = screenHome
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
	case "enter", "o", "l", "right":
		if rows := m.changeRows(); m.chCursor >= 0 && m.chCursor < len(rows) {
			r := rows[m.chCursor]
			switch {
			case r.url == "":
				m.flash = "no link for this change"
			default:
				if err := browser.Open(r.url); err != nil {
					m.flash = err.Error()
				} else {
					m.flash = "opened in browser"
				}
			}
		}
	}
	m.chClamp()
	return m, nil
}

// changesLines renders the screen body and the cursor's line.
func (m Model) changesLines() ([]string, int) {
	rows := m.changeRows()
	switch {
	case m.snap == nil:
		return []string{sDim.Render("  loading…")}, 0
	case m.since.IsZero():
		return []string{sDim.Render("  First visit: oryn is learning your baseline. Changes appear here from your next visit.")}, 0
	case len(rows) == 0:
		return []string{sOK.Render("  Nothing new since you last looked.")}, 0
	}
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, sTitle.Render("CHANGES")+sDim.Render(fmt.Sprintf("  %d rows · most severe first", len(rows))), "")
	cur := 0
	for i, r := range rows {
		marker := "  "
		if i == m.chCursor {
			marker = sAccent.Render("▸ ")
			cur = len(lines)
		}
		lines = append(lines, marker+r.line)
	}
	return lines, cur
}
