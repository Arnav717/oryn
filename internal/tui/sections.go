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
	title     string
	hints     string
	open      func(m *Model)            // land on the section
	attention func(m *Model) []attnItem // what feeds Needs you; nil for none
	keys      func(m Model, k tea.KeyMsg) (tea.Model, tea.Cmd)
	body      func(m Model) string
}

var sections [numSections]section

// Filled in here because the handlers themselves switch sections, which a
// package-level initializer would reject as a cycle.
func init() {
	sections = [numSections]section{
		secHome: {
			title: "Home",
			hints: "tab switch · enter open · f fix · x dismiss · z snooze",
			open:  func(m *Model) { m.screen = screenHome },
			keys:  Model.handleHomeKey,
			body:  Model.homeBody,
		},
		secProjects: {
			title: "Projects",
			hints: "enter open · f find in folder · a add · d remove · ctrl+p commands",
			open: func(m *Model) {
				m.screen = screenProjects
				m.clampHome()
			},
			keys: Model.handleProjectsKey,
			body: func(m Model) string { return m.fullBox(m.projectLines()) },
		},
		secAgents: {
			title: "Agents",
			hints: "n new · a attach · v review · x stop · enter details",
			open: func(m *Model) {
				m.screen = screenAgents
				m.agCursor, m.agOffset, m.agOpen = 0, 0, ""
			},
			attention: agentAttention,
			keys:      Model.handleAgentsKey,
			body:      func(m Model) string { return m.fullBox(m.agentBody()) },
		},
		secDeploys: {
			title: "Deploys",
			hints: "enter open · p deploy · A accounts · ctrl+p commands",
			open: func(m *Model) {
				m.screen = screenDeploys
				m.dpCursor, m.dpOffset = 0, 0
				m.dpClamp()
			},
			attention: deployAttention,
			keys:      Model.handleDeployKey,
			body:      func(m Model) string { return m.fullBox(m.deployBody()) },
		},
		secGitHub: {
			title: "GitHub",
			hints: "enter open · ctrl+p commands",
			open: func(m *Model) {
				m.screen = screenGitHub
				m.ghCursor, m.ghOffset = 0, 0
				m.ghClamp()
			},
			attention: githubAttention,
			keys:      Model.handleGHKey,
			body: func(m Model) string {
				lines, _, _ := m.ghLines()
				return m.fullBox(lines[min(m.ghOffset, len(lines)):])
			},
		},
		secHealth: {
			title:     "Health",
			hints:     "tab pane · / search · ctrl+p commands",
			open:      func(m *Model) { m.openScope("") },
			attention: healthAttention,
			keys:      Model.handlePaneKey,
			body:      Model.panes,
		},
		secActivity: {
			title: "Activity",
			hints: "f filter · enter open · ctrl+p commands",
			open: func(m *Model) {
				m.screen = screenActivity
				m.resetActivity()
			},
			keys: Model.handleActivityKey,
			body: Model.activityBody,
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
	case screenActivity:
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
	return !m.adding && !m.confirmRm && !m.tokenEntry && m.spawn == nil && m.finder == nil && m.review == nil && m.discard == nil && m.shipAsk == nil && m.screen != screenPicker && m.screen != screenDeployRun
}

// sectionKey maps a number key to its tab.
func sectionKey(k tea.KeyMsg) (sectionID, bool) {
	s := k.String()
	if len(s) != 1 || s[0] < '1' || s[0] >= '1'+byte(numSections) {
		return 0, false
	}
	return sectionID(s[0] - '1'), true
}

// tabsLine is the top row: name, tabs, time. On a narrow terminal the time
// goes first, then the titles of the tabs you are not on (their numbers stay).
func (m Model) tabsLine() string {
	active := m.section()
	render := func(short bool) string {
		parts := make([]string, numSections)
		for id := range numSections {
			switch {
			case id == active:
				parts[id] = sTab.Render(sections[id].title)
			case short:
				parts[id] = sTabOff.Render(fmt.Sprint(id + 1))
			default:
				parts[id] = sTabOff.Render(sections[id].title)
			}
		}
		return " " + sLogo.Render("◆ oryn") + "  " + strings.Join(parts, "")
	}
	right := m.now.Format("15:04") + " "
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
	return left + strings.Repeat(" ", gap) + sDim.Render(right)
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
