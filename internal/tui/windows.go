package tui

import (
	"fmt"
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
	winAgents
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
	rows    func(m *Model) ([]winRow, string) // rows and a short title summary
}

var homeWindows [numWins]windowDef

func init() {
	homeWindows = [numWins]windowDef{
		winNeeds:   {"NEEDS YOU", secHome, needsRows},
		winAgents:  {"AGENTS", secAgents, agentRows},
		winDeploys: {"DEPLOYS", secDeploys, deployWinRows},
		winGitHub:  {"GITHUB", secGitHub, githubWinRows},
	}
}

func needsRows(m *Model) ([]winRow, string) {
	if len(m.cfg.Projects) == 0 {
		return []winRow{{text: sDim.Render("Nothing watched yet. Press 2 to add a project."), to: target{section: secProjects}}}, ""
	}
	if m.snap == nil {
		return []winRow{{text: sDim.Render("loading…")}}, ""
	}
	items := m.needsYou()
	if len(items) == 0 {
		return []winRow{{text: sOK.Render("✓ nothing needs you right now")}}, ""
	}
	rows := make([]winRow, len(items))
	for i, it := range items {
		rows[i] = winRow{m.attnRow(it), it.to}
	}
	return rows, fmt.Sprint(len(items))
}

func agentRows(*Model) ([]winRow, string) {
	return []winRow{{text: sDim.Render("Not built yet.")}}, ""
}

func deployWinRows(m *Model) ([]winRow, string) {
	if m.snap == nil {
		return []winRow{{text: sDim.Render("loading…")}}, ""
	}
	var rows []winRow
	for _, t := range m.snap.Deploys {
		p := t.Link.Project
		name := pad(filepath.Base(p), 14) + " " + pad(t.Link.Platform, 11)
		to := target{project: p, tab: tabDeploys}
		if note := targetNote(t); note != "" {
			rows = append(rows, winRow{sDim.Render("? " + name + note), to})
			continue
		}
		d, ok := t.Latest("production")
		if !ok && len(t.Deployments) > 0 {
			d, ok = t.Deployments[0], true
		}
		if !ok {
			rows = append(rows, winRow{sDim.Render("? " + name + "no deployments"), to})
			continue
		}
		rows = append(rows, winRow{deployDot(d.State) + " " + name + pad(d.State, 9) + sDim.Render(shortAgo(m.now.Sub(d.Created))), to})
	}
	if len(rows) == 0 {
		return []winRow{{text: sDim.Render("No deploys linked yet. Open a project to link one.")}}, ""
	}
	_, live := m.deployCounts()
	return rows, fmt.Sprintf("%d live", live)
}

func githubWinRows(m *Model) ([]winRow, string) {
	prs := m.ghPRs()
	if len(prs) == 0 {
		return []winRow{{text: m.ghHomeSummary()}}, ""
	}
	rows := make([]winRow, len(prs))
	for i, p := range prs {
		rows[i] = winRow{ciCell(p) + " " + pad(p.Ref(), 30) + " " + sDim.Render(p.Title), target{url: p.URL}}
	}
	review, _, _ := m.ghSummary()
	return rows, fmt.Sprintf("%d to review", review)
}

// homeBody lays the windows out to fit: two by two when there's room, stacked
// on a narrow terminal, and only the focused one when they'd be too short to
// read.
func (m Model) homeBody() string {
	avail := m.h - m.chrome() - 1 // the health line
	const minStacked = 4
	var grid string
	switch {
	case m.w >= 100:
		lw, rw := m.w/2, m.w-m.w/2
		th, bh := avail/2, avail-avail/2
		grid = lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top, m.window(winNeeds, lw, th), m.window(winAgents, rw, th)),
			lipgloss.JoinHorizontal(lipgloss.Top, m.window(winDeploys, lw, bh), m.window(winGitHub, rw, bh)))
	case avail/int(numWins) >= minStacked:
		parts := make([]string, numWins)
		left := avail
		for w := homeWin(0); w < numWins; w++ {
			h := left / int(numWins-w)
			left -= h
			parts[w] = m.window(w, m.w, h)
		}
		grid = lipgloss.JoinVertical(lipgloss.Left, parts...)
	default:
		grid = m.window(m.win, m.w, avail)
	}
	return lipgloss.JoinVertical(lipgloss.Left, grid, m.healthLine())
}

func (m Model) window(w homeWin, width, height int) string {
	cw, ch := width-2, height-2
	rows, summary := homeWindows[w].rows(&m)
	title := sTitle.Render(homeWindows[w].title)
	if summary != "" {
		title += sDim.Render("  " + summary)
	}
	lines := []string{clip(title, cw)}

	vis := max(1, ch-1)
	cur := max(0, min(m.winCur[w], len(rows)-1))
	off := max(0, cur-vis+1)
	focused := w == m.win
	for i := off; i < min(len(rows), off+vis); i++ {
		marker := "  "
		if focused && i == cur {
			marker = sAccent.Render("▸ ")
		}
		lines = append(lines, clip(marker+rows[i].text, cw))
	}
	return paneBox(focused).Width(cw).Height(ch).Render(strings.Join(lines, "\n"))
}

func (m Model) healthLine() string {
	if m.snap == nil {
		return sDim.Render(" Health: checking…")
	}
	return clip(" "+sDim.Render("Health: ")+strings.Join([]string{
		vulnSummary(m.total.Vulns, m.total.Crit, m.total.High),
		sWarn.Render(fmt.Sprintf("%d outdated", m.total.Outdated)),
		m.upstreamPart(),
	}, sDim.Render(" · "))+sDim.Render("   press 6"), m.w)
}

func (m Model) handleHomeKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows, _ := homeWindows[m.win].rows(&m)
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
