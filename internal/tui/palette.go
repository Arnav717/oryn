package tui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// palItem is one command in the ctrl+p palette.
type palItem struct {
	group, title, key string
	run               func(m Model) (tea.Model, tea.Cmd)
}

func (m Model) palItems() []palItem {
	var items []palItem
	for id := range numSections {
		items = append(items, palItem{"Go to", sections[id].title, string(rune('1' + id)), func(m Model) (tea.Model, tea.Cmd) {
			m.openSection(id)
			return m, nil
		}})
	}
	for _, p := range m.cfg.Projects {
		items = append(items, palItem{"Projects", filepath.Base(p), "", func(m Model) (tea.Model, tea.Cmd) {
			m.openScope(p)
			return m, nil
		}})
	}
	return append(items,
		palItem{"Actions", "New agent", "n", Model.openSpawn},
		palItem{"Actions", "Refresh", "r", func(m Model) (tea.Model, tea.Cmd) { return m, m.startRefresh() }},
		palItem{"Actions", "Connect accounts", "A", Model.palAccounts},
		palItem{"Actions", "Find projects in a folder", "f", Model.openFinder},
		palItem{"Actions", "Add a project", "a", func(m Model) (tea.Model, tea.Cmd) {
			m.openSection(secProjects)
			return m.startAdd(m.cwd)
		}},
		palItem{"Actions", "Help", "?", func(m Model) (tea.Model, tea.Cmd) { m.showHelp = true; return m, nil }},
		palItem{"Actions", "Quit", "q", func(m Model) (tea.Model, tea.Cmd) { return m, tea.Quit }},
	)
}

func (m Model) palAccounts() (tea.Model, tea.Cmd) { return m.openAccounts("") }

// palMatches filters by the typed letters, in order.
func (m Model) palMatches() []palItem {
	q := strings.ToLower(strings.TrimSpace(m.palInput.Value()))
	return slices.DeleteFunc(m.palItems(), func(it palItem) bool {
		return !subsequence(strings.ToLower(it.title), q)
	})
}

func (m Model) openPalette() (tea.Model, tea.Cmd) {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "Search"
	m.palOpen, m.palInput, m.palCur = true, in, 0
	return m, m.palInput.Focus()
}

func (m Model) handlePaletteKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.palMatches()
	switch k.Type {
	case tea.KeyEsc, tea.KeyCtrlP:
		m.palOpen = false
		return m, nil
	case tea.KeyUp, tea.KeyCtrlK:
		m.palCur = max(m.palCur-1, 0)
		return m, nil
	case tea.KeyDown, tea.KeyCtrlJ:
		m.palCur = min(m.palCur+1, len(items)-1)
		return m, nil
	case tea.KeyEnter:
		m.palOpen = false
		if m.palCur >= 0 && m.palCur < len(items) {
			return items[m.palCur].run(m)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.palInput, cmd = m.palInput.Update(k)
	m.palCur = 0
	return m, cmd
}

// paletteBody is the palette centred in the body area.
func (m Model) paletteBody() string {
	bodyH := m.h - m.chrome()
	w := min(64, m.w-4)
	inner := w - 4

	row := func(l, r string) string {
		gap := inner - 3 - lipgloss.Width(l) - lipgloss.Width(r)
		return "  " + l + strings.Repeat(" ", max(1, gap)) + r
	}
	header := []string{m.palInput.View(), ""}

	items := m.palMatches()
	vis := max(3, min(22, bodyH-len(header)-3)) // rows for items and group titles
	type line struct {
		text string
		item int // -1 for a group title
	}
	var all []line
	group := ""
	for i, it := range items {
		if it.group != group {
			group = it.group
			all = append(all, line{sGroup.Render(group), -1})
		}
		all = append(all, line{"", i})
	}
	at := slices.IndexFunc(all, func(l line) bool { return l.item == m.palCur })
	off := max(0, at-vis+1)

	lines := slices.Clone(header)
	for _, l := range all[off:min(len(all), off+vis)] {
		switch {
		case l.item < 0:
			lines = append(lines, l.text)
		case l.item == m.palCur:
			lines = append(lines, selBar(strings.TrimPrefix(row(items[l.item].title, items[l.item].key), "  "), inner))
		default:
			lines = append(lines, row(items[l.item].title, sDim.Render(items[l.item].key)))
		}
	}
	if len(items) == 0 {
		lines = append(lines, sDim.Render("no matches"))
	}

	block := strings.Split(panel("Commands", lines, w, len(lines)+2, true), "\n")
	top := max(0, (bodyH-len(block))/2)
	left := strings.Repeat(" ", max(0, (m.w-w)/2))
	out := make([]string, 0, bodyH)
	for range top {
		out = append(out, "")
	}
	for _, b := range block {
		out = append(out, left+b)
	}
	if len(out) > bodyH {
		out = out[:bodyH]
	}
	for len(out) < bodyH {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}
