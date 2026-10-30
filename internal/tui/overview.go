package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Arnav717/oryn/internal/app"
)

// projWin is a window on a project's front page. Each is a short summary that
// expands into the full tab named by projWindows.
type projWin int

const (
	pwNeeds projWin = iota
	pwAgents
	pwDeploys
	pwGitHub
	pwHealth
	pwActivity
	numPWins
)

type projWindowDef struct {
	title string
	tab   projTab // what enter expands to; tabOverview for none
	rows  func(m *Model) []winRow
}

var projWindows [numPWins]projWindowDef

func init() {
	projWindows = [numPWins]projWindowDef{
		pwNeeds:    {"Needs you", tabOverview, projNeedsRows},
		pwAgents:   {"Agents", tabAgents, projAgentRows},
		pwDeploys:  {"Live", tabDeploys, projDeployRows},
		pwGitHub:   {"GitHub", tabGitHub, projGitHubRows},
		pwHealth:   {"Health", tabHealth, projHealthRows},
		pwActivity: {"Activity", tabActivity, projActivityRows},
	}
}

// windowForTab is the window that expands into t.
func windowForTab(t projTab) (projWin, bool) {
	for w, d := range projWindows {
		if d.tab == t && t != tabOverview {
			return projWin(w), true
		}
	}
	return 0, false
}

func projNeedsRows(m *Model) []winRow {
	items := m.needsFor(m.scope)
	if len(items) == 0 {
		return []winRow{{text: sOK.Render("✓") + " " + sDim.Render("all clear")}}
	}
	rows := make([]winRow, len(items))
	for i, it := range items {
		rows[i] = winRow{it.render(), it.to, it.fix}
	}
	return rows
}

func projAgentRows(m *Model) []winRow {
	return []winRow{{text: m.agentSummary(m.scope)}}
}

func projDeployRows(m *Model) []winRow {
	var rows []winRow
	if m.snap != nil {
		for _, t := range m.snap.Deploys {
			if t.Link.Project == m.scope {
				rows = append(rows, winRow{text: m.liveCell(t)})
			}
		}
	}
	if len(rows) == 0 {
		return []winRow{{text: sDim.Render("not deployed yet, enter to link a platform")}}
	}
	return rows
}

func projGitHubRows(m *Model) []winRow {
	d, why := m.repoState()
	if d == nil {
		if len(why) == 0 {
			return nil
		}
		return []winRow{{text: why[0]}}
	}
	rows := []winRow{{text: strings.Join([]string{
		sBold.Render(orDash(d.DefaultBranch)) + " " + ciWord(d.DefaultCI),
		fmt.Sprintf("%d PRs", d.OpenPRs),
		fmt.Sprintf("%d issues", d.OpenIssues),
	}, sDim.Render(" · "))}}
	for _, c := range d.Commits[:min(3, len(d.Commits))] {
		msg, _, _ := strings.Cut(c.Message, "\n")
		rows = append(rows, winRow{text: msg + sDim.Render("  "+c.Author+", "+humanAge(c.At))})
	}
	return rows
}

func projHealthRows(m *Model) []winRow {
	st, ok := m.stats[m.scope]
	if !ok {
		return []winRow{{text: sDim.Render("loading…")}}
	}
	vulns := sOK.Render("✓") + " no known vulnerabilities"
	if st.Vulns > 0 {
		vulns = sBad.Render(fmt.Sprintf("%d vulnerabilities", st.Vulns)) + sDim.Render(fmt.Sprintf(" · %d critical · %d high", st.Crit, st.High))
	}
	return []winRow{
		{text: fmt.Sprintf("%d dependencies", st.Deps) + sDim.Render(fmt.Sprintf(" · %d direct · %d outdated", st.Direct, st.Outdated))},
		{text: vulns},
	}
}

func projActivityRows(m *Model) []winRow {
	lines, _, _ := m.activity()
	rows := make([]winRow, len(lines))
	for i, l := range lines {
		rows[i] = winRow{text: l}
	}
	return rows
}

// winRect is where a window sits in the body, so a click can find it.
type winRect struct {
	x, y, w, h int
	win        projWin
}

// gridLayout places the windows: two columns when wide, one when narrow, and
// only the focused window when they'd be too short to read.
func (m Model) gridLayout() []winRect {
	avail := m.h - m.chrome()
	const minHeight = 4 // the frame and two rows
	cols := 1
	if m.w >= 100 {
		cols = 2
	}
	rows := (int(numPWins) + cols - 1) / cols
	if avail/rows < minHeight {
		return []winRect{{0, 0, m.w, avail, m.pwin}}
	}
	var out []winRect
	y, left := 0, avail
	for r := 0; r < rows; r++ {
		h := left / (rows - r)
		left -= h
		x := 0
		for c := 0; c < cols; c++ {
			i := r*cols + c
			if i >= int(numPWins) {
				break
			}
			w := m.w / cols
			if c == cols-1 {
				w = m.w - x
			}
			out = append(out, winRect{x, y, w, h, projWin(i)})
			x += w
		}
		y += h
	}
	return out
}

func (m Model) overviewBody() string {
	var bands []string
	var band []string
	lastY := -1
	for _, r := range m.gridLayout() {
		if r.y != lastY && len(band) > 0 {
			bands = append(bands, lipgloss.JoinHorizontal(lipgloss.Top, band...))
			band = nil
		}
		lastY = r.y
		band = append(band, m.projWindow(r))
	}
	bands = append(bands, lipgloss.JoinHorizontal(lipgloss.Top, band...))
	return lipgloss.JoinVertical(lipgloss.Left, bands...)
}

func (m Model) projWindow(r winRect) string {
	rows := projWindows[r.win].rows(&m)
	focused := r.win == m.pwin
	cw, vis := r.w-4, max(1, r.h-2)
	cur := max(0, min(m.pwCur[r.win], len(rows)-1))
	off := max(0, cur-vis+1)
	lines := make([]string, 0, vis)
	for i := off; i < min(len(rows), off+vis); i++ {
		marker := "  "
		if focused && i == cur {
			marker = sAccent.Render("▸ ")
		}
		lines = append(lines, clip(marker+rows[i].text, cw))
	}
	return panel(projWindows[r.win].title, lines, r.w, r.h, focused)
}

// expandWin opens the focused window's full tab. Needs you has none, so
// enter acts on the selected line instead.
func (m Model) expandWin() Model {
	d := projWindows[m.pwin]
	if d.tab != tabOverview {
		m.setTab(d.tab)
		return m
	}
	rows := d.rows(&m)
	if cur := max(0, min(m.pwCur[m.pwin], len(rows)-1)); len(rows) > 0 && rows[cur].to != (target{}) {
		m.goTo(rows[cur].to)
	}
	return m
}

// gridActive: the window grid is what's on screen and takes clicks.
func (m Model) gridActive() bool {
	return m.screen == screenProject && m.tab == tabOverview && m.detail == "" && m.navFree() && !m.cmdMode &&
		!m.palOpen && !m.showHelp && m.deployAsk == nil && m.delAsk == nil && m.stopAsk == nil
}

func (m Model) handleClick(x, y int) Model {
	if !m.gridActive() {
		return m
	}
	y -= m.chrome() - 1 // the lines above the body
	for _, r := range m.gridLayout() {
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			m.pwin = r.win
			return m.expandWin()
		}
	}
	return m
}

func (m Model) handleOverviewKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := projWindows[m.pwin].rows(&m)
	cur := max(0, min(m.pwCur[m.pwin], len(rows)-1))
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		m.openSection(secProjects)
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "]":
		m.switchTab(1)
	case "[":
		m.switchTab(-1)
	case ":", "/":
		return m.openCommand()
	case "tab", "l", "right":
		m.pwin = (m.pwin + 1) % numPWins
	case "shift+tab", "h", "left":
		m.pwin = (m.pwin + numPWins - 1) % numPWins
	case "j", "down":
		m.pwCur[m.pwin] = min(cur+1, len(rows)-1)
	case "k", "up":
		m.pwCur[m.pwin] = max(cur-1, 0)
	case "g":
		m.setTab(tabGitHub)
	case "D":
		m.setTab(tabDeploys)
	case "A":
		return m.openAccounts("")
	case "p":
		return m.deployFromOverview()
	case "f":
		if len(rows) > 0 && rows[cur].fix != "" {
			return m.openSpawnWith(m.scope, rows[cur].fix)
		}
		m.flash = "nothing here for an agent to fix"
	case "enter":
		return m.expandWin(), nil
	}
	return m, nil
}

// liveCell is one deploy target on the overview.
func (m Model) liveCell(t app.DeployTarget) string {
	name := pad(t.Link.Platform, 12)
	if note := targetNote(t); note != "" {
		return sDim.Render("· " + name + note)
	}
	d, ok := t.Latest("production")
	if !ok {
		return sDim.Render("· " + name + "no deployments")
	}
	return deployDot(d.State) + " " + name + pad(d.State, 9) + orDefault(t.LiveURL, d.URL) + sDim.Render("  "+shortAgo(m.now.Sub(d.Created)))
}

// deployFromOverview deploys straight away when there is one place to deploy
// to. With several, the Deploys tab is where you choose.
func (m Model) deployFromOverview() (tea.Model, tea.Cmd) {
	var linked []app.DeployTarget
	if m.snap != nil {
		for _, t := range m.snap.Deploys {
			if t.Link.Project == m.scope && t.Link.Linked() {
				linked = append(linked, t)
			}
		}
	}
	switch len(linked) {
	case 0:
		m.setTab(tabDeploys)
		m.flash = "link a platform first (enter), then press p to deploy"
		return m, nil
	case 1:
		return m.askDeploy(linked[0])
	}
	m.setTab(tabDeploys)
	m.flash = "linked to " + joinPlatforms(linked) + ": pick one and press p"
	return m, nil
}

func joinPlatforms(ts []app.DeployTarget) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.Link.Platform
	}
	return strings.Join(names, " and ")
}
