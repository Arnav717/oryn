package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Arnav717/oryn/internal/deploy"
)

// sectionID is a top-level tab. The number keys switch between them from
// anywhere, so a new feature adds a const here and a row in init below.
type sectionID int

const (
	secHome sectionID = iota
	secProjects
	secAgents
	secDeploys
	secGitHub
	secHealth
	secActivity
	numSections
)

type section struct {
	title string
	hints string
	open  func(m *Model)        // land on the section
	badge func(m *Model) string // short count on the tab, "" for none
	keys  func(m Model, k tea.KeyMsg) (tea.Model, tea.Cmd)
	body  func(m Model) string
}

var sections [numSections]section

// Filled in here because the handlers themselves switch sections, which a
// package-level initializer would reject as a cycle.
func init() {
	sections = [numSections]section{
		secHome: {
			title: "Home",
			hints: "enter projects · c activity · D deploys · A accounts · r refresh · ? help · q quit",
			open:  func(m *Model) { m.screen = screenHome },
			badge: func(*Model) string { return "" },
			keys:  Model.handleHomeKey,
			body:  func(m Model) string { return m.fullBox(m.homeLines()) },
		},
		secProjects: {
			title: "Projects",
			hints: "j/k move · enter open · a add · d remove · c activity · D deploys · A accounts · esc home · r refresh · ? help · q quit",
			open: func(m *Model) {
				m.screen = screenProjects
				m.clampHome()
			},
			badge: func(*Model) string { return "" },
			keys:  Model.handleProjectsKey,
			body:  func(m Model) string { return m.fullBox(m.projectLines()) },
		},
		secAgents: {
			title: "Agents",
			hints: "esc home · r refresh · ? help · q quit",
			open:  func(m *Model) { m.screen = screenAgents },
			badge: func(*Model) string { return "" },
			keys:  Model.handleAgentsKey,
			body:  func(m Model) string { return m.fullBox(agentLines()) },
		},
		secDeploys: {
			title: "Deploys",
			hints: "j/k move · enter open/link · p deploy · d delete deploy · X delete project · u unlink · A accounts · esc home · r refresh · ? help · q quit",
			open: func(m *Model) {
				m.screen = screenDeploys
				m.dpCursor, m.dpOffset = 0, 0
				m.dpClamp()
			},
			badge: func(m *Model) string {
				if failing, _ := m.deployCounts(); failing > 0 {
					return sBad.Render(fmt.Sprint(failing))
				}
				return ""
			},
			keys: Model.handleDeployKey,
			body: func(m Model) string { return m.fullBox(m.deployBody()) },
		},
		secGitHub: {
			title: "GitHub",
			hints: "j/k move · enter open in browser · esc home · r refresh · ? help · q quit",
			open: func(m *Model) {
				m.screen = screenGitHub
				m.ghCursor, m.ghOffset = 0, 0
				m.ghClamp()
			},
			badge: func(m *Model) string {
				if review, _, failing := m.ghSummary(); failing > 0 {
					return sBad.Render(fmt.Sprint(review + failing))
				} else if review > 0 {
					return sAccent.Render(fmt.Sprint(review))
				}
				return ""
			},
			keys: Model.handleGHKey,
			body: func(m Model) string {
				lines, _, _ := m.ghLines()
				return m.fullBox(lines[min(m.ghOffset, len(lines)):])
			},
		},
		secHealth: {
			title: "Health",
			hints: "tab pane · j/k move · enter open · / search · esc home · r refresh · ? help · q quit",
			open:  func(m *Model) { m.openScope("") },
			badge: func(m *Model) string {
				if m.total.Crit > 0 {
					return sDim.Render(fmt.Sprint(m.total.Crit))
				}
				return ""
			},
			keys: Model.handlePaneKey,
			body: Model.panes,
		},
		secActivity: {
			title: "Activity",
			hints: "j/k move · enter open in browser · e expand/fold · esc home · r refresh · ? help · q quit",
			open: func(m *Model) {
				m.screen = screenChanges
				m.chCursor, m.chOffset, m.chExpand = 0, 0, false
				m.chClamp()
			},
			badge: func(m *Model) string {
				if n := len(m.sinceEvents()); n > 0 {
					return sAccent.Render(fmt.Sprint(n))
				}
				return ""
			},
			keys: Model.handleChangesKey,
			body: func(m Model) string {
				lines, _ := m.changesLines()
				return m.fullBox(lines[min(m.chOffset, len(lines)):])
			},
		},
	}
}

// section is the tab the current screen belongs to. Sub-screens (accounts, the
// link picker, a running deploy) stay under whatever opened them.
func (m Model) section() sectionID {
	switch m.screen {
	case screenProjects, screenProject:
		return secProjects
	case screenAgents:
		return secAgents
	case screenDeploys:
		return secDeploys
	case screenGitHub:
		return secGitHub
	case screenHealth:
		return secHealth
	case screenChanges:
		return secActivity
	case screenAccounts:
		return Model{screen: m.acctBack}.section()
	case screenPicker:
		return Model{screen: m.pickBack}.section()
	case screenDeployRun:
		return Model{screen: m.runBack}.section()
	}
	return secHome
}

func (m *Model) openSection(id sectionID) {
	m.detail = ""
	sections[id].open(m)
}

// navFree reports whether number keys may switch tabs: not while typing, and
// not out of a flow that has to finish first.
func (m Model) navFree() bool {
	return !m.adding && !m.confirmRm && !m.tokenEntry && m.screen != screenPicker && m.screen != screenDeployRun
}

// sectionKey maps a number key to its tab.
func sectionKey(k tea.KeyMsg) (sectionID, bool) {
	s := k.String()
	if len(s) != 1 || s[0] < '1' || s[0] >= '1'+byte(numSections) {
		return 0, false
	}
	return sectionID(s[0] - '1'), true
}

// tabsLine is the top row: logo, the tabs, the clock. On a narrow terminal the
// clock goes first, then the titles of the tabs you are not on.
func (m Model) tabsLine() string {
	active := m.section()
	render := func(short bool) string {
		parts := make([]string, numSections)
		for id := range numSections {
			s := sections[id]
			num, title := fmt.Sprint(id+1), " "+s.title
			if id == active {
				parts[id] = sAccent.Render(num) + sAccent.Bold(true).Underline(true).Render(title)
			} else if short {
				parts[id] = sDim.Render(num)
			} else {
				parts[id] = sDim.Render(num + title)
			}
			if b := s.badge(&m); b != "" {
				parts[id] += " " + b
			}
		}
		return sLogo.Render("ORYN") + "  " + strings.Join(parts, "  ")
	}
	right := m.now.Format("Mon 02 Jan 15:04:05")
	if m.loading {
		right = m.spin.View() + " " + right
	}
	left := render(false)
	if lipgloss.Width(left) > m.w {
		left = render(true)
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return clip(left, m.w)
	}
	return left + strings.Repeat(" ", gap) + sAccent.Render(right)
}

func (m Model) handleAgentsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		m.openSection(secHome)
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	}
	return m, nil
}

func agentLines() []string {
	return []string{
		sTitle.Render("AGENTS"),
		"",
		"  Not built yet.",
		sDim.Render("  This tab will show every AI agent working across your projects: what it's doing and what it needs from you."),
	}
}

// deployCell is a project's latest production deploy in one short cell.
func (m Model) deployCell(project string) string {
	if m.snap == nil {
		return sDim.Render("…")
	}
	var best deploy.Deployment
	platform := ""
	for _, t := range m.snap.Deploys {
		if t.Link.Project != project {
			continue
		}
		if d, ok := t.Latest("production"); ok && (platform == "" || d.Created.After(best.Created)) {
			best, platform = d, t.Link.Platform
		}
	}
	if platform == "" {
		return sDim.Render("-")
	}
	return deployDot(best.State) + " " + platform + sDim.Render(" "+shortAgo(m.now.Sub(best.Created)))
}
