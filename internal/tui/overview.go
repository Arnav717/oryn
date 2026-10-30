package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/app"
)

// overviewRows is a project's front page: one selectable line per thing that
// matters, each opening the tab with the detail.
func (m Model) overviewRows() []winRow {
	p := m.scope
	label := func(s string) string { return sDim.Render(pad(s, 8)) }
	var rows []winRow

	for i, it := range m.needsFor(p) {
		head := pad("", 8)
		if i == 0 {
			head = label("Needs")
		}
		rows = append(rows, winRow{head + it.render(), it.to})
	}

	rows = append(rows, winRow{label("Agents") + m.agentSummary(p), target{project: p, tab: tabAgents}})

	deploys := target{project: p, tab: tabDeploys}
	shown := false
	if m.snap != nil {
		for _, t := range m.snap.Deploys {
			if t.Link.Project != p {
				continue
			}
			head := pad("", 8)
			if !shown {
				head = label("Live")
			}
			shown = true
			rows = append(rows, winRow{head + m.liveCell(t), deploys})
		}
	}
	if !shown {
		rows = append(rows, winRow{label("Live") + sDim.Render("not deployed yet, enter to link a platform"), deploys})
	}

	github := target{project: p, tab: tabGitHub, sub: svOverview}
	if d, why := m.repoState(); d != nil {
		rows = append(rows, winRow{label("GitHub") + strings.Join([]string{
			sBold.Render(orDash(d.DefaultBranch)) + " " + ciWord(d.DefaultCI),
			fmt.Sprintf("%d PRs", d.OpenPRs),
			fmt.Sprintf("%d issues", d.OpenIssues),
		}, sDim.Render(" · ")), github})
	} else if len(why) > 0 {
		rows = append(rows, winRow{label("GitHub") + why[0], github})
	}

	if d := m.repoDataFor(p); d != nil {
		for i, c := range d.Commits[:min(3, len(d.Commits))] {
			head := pad("", 8)
			if i == 0 {
				head = label("Recent")
			}
			msg, _, _ := strings.Cut(c.Message, "\n")
			rows = append(rows, winRow{head + msg + sDim.Render("  "+c.Author+", "+humanAge(c.At)), target{url: c.URL}})
		}
	}
	return rows
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

func (m Model) overviewBody() string {
	rows := m.overviewRows()
	vis := max(1, m.h-m.chrome()-2)
	cur := max(0, min(m.ovCur, len(rows)-1))
	lines := make([]string, 0, vis)
	for i := max(0, cur-vis+1); i < len(rows) && len(lines) < vis; i++ {
		marker := "  "
		if i == cur {
			marker = sAccent.Render("▸ ")
		}
		lines = append(lines, marker+rows[i].text)
	}
	return m.fullBox(lines)
}

func (m Model) handleOverviewKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.overviewRows()
	cur := max(0, min(m.ovCur, len(rows)-1))
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
	case "j", "down":
		m.ovCur = min(cur+1, len(rows)-1)
	case "k", "up":
		m.ovCur = max(cur-1, 0)
	case "g", "home":
		m.setTab(tabGitHub)
	case "D":
		m.setTab(tabDeploys)
	case "A":
		return m.openAccounts("")
	case "p":
		return m.deployFromOverview()
	case "enter":
		if len(rows) > 0 {
			m.goTo(rows[cur].to)
		}
	}
	return m, nil
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
