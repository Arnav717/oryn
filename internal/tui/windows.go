package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// homeWin is a window on the Home tab. Each is a short list that opens into
// the full tab named by homeWindows.
type homeWin int

const (
	winNeeds homeWin = iota
	winDeploys
	winGitHub
	numWins
)

// winRow is a line in a window. A zero target opens the window's own tab.
type winRow struct {
	text string
	to   target
}

type windowDef struct {
	title   string
	section sectionID
	rows    func(m *Model) []winRow
}

var homeWindows [numWins]windowDef

func init() {
	homeWindows = [numWins]windowDef{
		winNeeds:   {"Needs you", secHome, needsRows},
		winDeploys: {"Deploys", secDeploys, deployWinRows},
		winGitHub:  {"GitHub", secGitHub, githubWinRows},
	}
}

func needsRows(m *Model) []winRow {
	if len(m.cfg.Projects) == 0 {
		return []winRow{{text: sDim.Render("Nothing watched yet. Press 2 to add a project."), to: target{section: secProjects}}}
	}
	if m.snap == nil {
		return []winRow{{text: sDim.Render("loading…")}}
	}
	items := m.needsYou()
	if len(items) == 0 {
		return []winRow{{text: sOK.Render("✓") + " " + sDim.Render("all clear")}}
	}
	rows := make([]winRow, len(items))
	for i, it := range items {
		rows[i] = winRow{m.attnRow(it), it.to}
	}
	return rows
}

// deployWinRows shows what's actually deployed. Links with nothing on them
// are set-up detail for the Deploys tab.
func deployWinRows(m *Model) []winRow {
	if m.snap == nil {
		return []winRow{{text: sDim.Render("loading…")}}
	}
	var rows []winRow
	for _, t := range m.snap.Deploys {
		d, ok := t.Latest("production")
		if !ok && len(t.Deployments) > 0 {
			d, ok = t.Deployments[0], true
		}
		if !ok {
			continue
		}
		p := t.Link.Project
		rows = append(rows, winRow{deployDot(d.State) + " " + pad(filepath.Base(p), 14) + " " + sDim.Render(pad(t.Link.Platform, 11)+shortAgo(m.now.Sub(d.Created))),
			target{project: p, tab: tabDeploys}})
	}
	if len(rows) == 0 {
		return []winRow{{text: sDim.Render("nothing deployed")}}
	}
	return rows
}

func githubWinRows(m *Model) []winRow {
	prs := m.ghPRs()
	if len(prs) == 0 {
		return []winRow{{text: m.ghHomeSummary()}}
	}
	rows := make([]winRow, len(prs))
	for i, p := range prs {
		_, ref, _ := strings.Cut(p.Ref(), "/")
		rows[i] = winRow{pad(ref, 26) + " " + sDim.Render(p.Title), target{url: p.URL}}
	}
	return rows
}

// homeBody lays the windows out to fit: Needs you beside the others when
// there's room, stacked on a narrow terminal, and only the focused one when
// they'd be too short to read.
func (m Model) homeBody() string {
	avail := m.h - m.chrome()
	const minStacked = 6 // margin, title, gap and two rows
	switch {
	case m.w >= 100:
		lw, rw := m.w/2, m.w-m.w/2
		th, bh := avail/2, avail-avail/2
		return lipgloss.JoinHorizontal(lipgloss.Top, m.window(winNeeds, lw, avail),
			lipgloss.JoinVertical(lipgloss.Left, m.window(winDeploys, rw, th), m.window(winGitHub, rw, bh)))
	case avail/int(numWins) >= minStacked:
		parts := make([]string, numWins)
		left := avail
		for w := homeWin(0); w < numWins; w++ {
			h := left / int(numWins-w)
			left -= h
			parts[w] = m.window(w, m.w, h)
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	return m.window(m.win, m.w, avail)
}

func (m Model) window(w homeWin, width, height int) string {
	cw, ch := width-2, height-2
	rows := homeWindows[w].rows(&m)
	focused := w == m.win
	title := sDim.Bold(true).Render(homeWindows[w].title)
	if focused {
		title = sAccent.Bold(true).Render(homeWindows[w].title)
	}
	lines := []string{clip(title, cw), ""}

	vis := max(1, ch-2)
	cur := max(0, min(m.winCur[w], len(rows)-1))
	off := max(0, cur-vis+1)
	for i := off; i < min(len(rows), off+vis); i++ {
		marker := "  "
		if focused && i == cur {
			marker = sAccent.Render("▸ ")
		}
		lines = append(lines, clip(marker+rows[i].text, cw))
	}
	if len(lines) > ch {
		lines = lines[:ch]
	}
	return box(focused).Width(cw).Height(ch).Render(strings.Join(lines, "\n"))
}

func (m Model) handleHomeKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := homeWindows[m.win].rows(&m)
	cur := max(0, min(m.winCur[m.win], len(rows)-1))
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "tab", "l", "right":
		m.win = (m.win + 1) % numWins
	case "shift+tab", "h", "left":
		m.win = (m.win + numWins - 1) % numWins
	case "j", "down":
		m.winCur[m.win] = min(cur+1, len(rows)-1)
	case "k", "up":
		m.winCur[m.win] = max(cur-1, 0)
	case "g", "home":
		m.winCur[m.win] = 0
	case "G", "end":
		m.winCur[m.win] = len(rows) - 1
	case "enter":
		if t := rows[cur].to; t != (target{}) {
			m.goTo(t)
		} else if s := homeWindows[m.win].section; s != secHome {
			m.openSection(s)
		}
	case "A":
		return m.openAccounts("")
	case "D":
		m.openSection(secDeploys)
	case "c":
		m.openSection(secActivity)
	}
	return m, nil
}
