package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/agents"
	"github.com/Arnav717/oryn/internal/config"
)

// agentPoll is how often session files are re-read. They're local, so this
// can be far quicker than the network refresh.
const agentPoll = 3 * time.Second

func readAgents() tea.Msg { return agentsMsg{agents.List(time.Now())} }

// agentProject names a session's project: the watched one it's in, else its
// folder.
func (m Model) agentProject(s agents.Session) string {
	if root := m.watchedRoot(s.Dir); root != "" {
		return filepath.Base(root)
	}
	return filepath.Base(s.Dir)
}

func agentTitle(s agents.Session) string {
	if s.Title != "" {
		return s.Title
	}
	return "untitled"
}

func agentAge(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	return shortAgo(d)
}

func agentDot(st agents.State) string {
	switch st {
	case agents.Waiting:
		return sWarn.Render("●")
	case agents.Working:
		return sAccent.Render("●")
	case agents.Failed:
		return sBad.Render("●")
	}
	return sDim.Render("○")
}

// agentList is the sessions in view: all of them, or one project's.
func (m Model) agentList() []agents.Session {
	if m.screen != screenProject {
		return m.agents
	}
	var out []agents.Session
	for _, s := range m.agents {
		if m.watchedRoot(s.Dir) == m.scope {
			out = append(out, s)
		}
	}
	return out
}

func agentAttention(m *Model) []attnItem {
	var out []attnItem
	for _, s := range m.agents {
		if s.State != agents.Waiting {
			continue
		}
		out = append(out, attnItem{sev: attnWarn, text: s.Tool + " is waiting on you: " + agentTitle(s), at: s.Updated,
			project: m.watchedRoot(s.Dir), name: filepath.Base(s.Dir), to: target{section: secAgents}})
	}
	return out
}

func agentWinRows(m *Model) []winRow {
	var rows []winRow
	for _, s := range m.agents {
		if s.Active() {
			rows = append(rows, winRow{agentDot(s.State) + " " + pad(m.agentProject(s), 14) + " " + pad(s.Tool, 9) + sDim.Render(agentTitle(s)), target{section: secAgents}})
		}
	}
	if len(rows) == 0 {
		return []winRow{{text: sDim.Render("none running")}}
	}
	return rows
}

// agentSummary is the Agents row on a project's overview.
func (m Model) agentSummary(project string) string {
	var waiting, working int
	for _, s := range m.agents {
		if m.watchedRoot(s.Dir) != project {
			continue
		}
		switch s.State {
		case agents.Waiting:
			waiting++
		case agents.Working:
			working++
		}
	}
	var parts []string
	if working > 0 {
		parts = append(parts, fmt.Sprintf("%d running", working))
	}
	if waiting > 0 {
		parts = append(parts, sWarn.Render(fmt.Sprintf("%d waiting", waiting)))
	}
	if len(parts) == 0 {
		return sDim.Render("none running")
	}
	return strings.Join(parts, sDim.Render(" · "))
}

// agentGroups splits sessions the way the tab shows them.
func agentGroups(list []agents.Session) (rows []agents.Session, heads map[int]string) {
	heads = map[int]string{}
	group := ""
	for _, s := range list {
		g := "Recent"
		switch s.State {
		case agents.Waiting:
			g = "Waiting on you"
		case agents.Working:
			g = "Running"
		}
		if g != group {
			group = g
			heads[len(rows)] = g
		}
		rows = append(rows, s)
	}
	return rows, heads
}

// agentLines is the list body, and the line each session sits on.
func (m Model) agentLines() ([]string, []int) {
	rows, heads := agentGroups(m.agentList())
	if len(rows) == 0 {
		return []string{"", sDim.Render("No agent sessions in the last week.")}, nil
	}
	cw := m.w - 2
	const nameW, toolW, stateW, ageW = 18, 10, 9, 6
	titleW := max(10, cw-2-nameW-toolW-stateW-ageW)
	lines := []string{}
	rowLine := make([]int, len(rows))
	for i, s := range rows {
		if h, ok := heads[i]; ok {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, sDim.Bold(true).Render(h))
		}
		marker := "  "
		if i == m.agCursor {
			marker = sAccent.Render("▸ ")
		}
		rowLine[i] = len(lines)
		lines = append(lines, marker+agentDot(s.State)+" "+pad(m.agentProject(s), nameW-3)+" "+pad(s.Tool, toolW)+
			pad(agentTitle(s), titleW)+pad(sDim.Render(s.State.String()), stateW)+sDim.Render(agentAge(m.now.Sub(s.Updated))))
	}
	return lines, rowLine
}

func (m Model) agentBody() []string {
	if m.agOpen != "" {
		for _, s := range m.agentList() {
			if s.ID == m.agOpen {
				return m.agentDetail(s)
			}
		}
	}
	lines, _ := m.agentLines()
	return lines[min(m.agOffset, len(lines)):]
}

func (m Model) agentDetail(s agents.Session) []string {
	row := func(k, v string) string { return sDim.Render(pad(k, 10)) + v }
	lines := []string{agentDot(s.State) + " " + sTitle.Render(agentTitle(s)), ""}
	if s.Needs != "" {
		lines = append(lines, row("Needs", s.Needs))
	}
	lines = append(lines,
		row("Status", s.State.String()+sDim.Render("  updated "+agentAge(m.now.Sub(s.Updated)))),
		row("Tool", s.Tool),
		row("Folder", config.ShortHome(s.Dir)),
	)
	if s.Tokens > 0 {
		lines = append(lines, row("Tokens", fmt.Sprint(s.Tokens)))
	}
	return append(lines, "", sDim.Render("Resume it"), "  cd "+config.ShortHome(s.Dir)+" && "+s.Resume())
}

func (m *Model) agClamp() {
	lines, rowLine := m.agentLines()
	n := len(rowLine)
	m.agCursor = max(0, min(m.agCursor, n-1))
	if m.h == 0 || n == 0 {
		m.agOffset = 0
		return
	}
	vis := max(1, m.h-m.chrome()-2)
	line := rowLine[m.agCursor]
	if m.agCursor == 0 {
		m.agOffset = 0
	}
	if line < m.agOffset {
		m.agOffset = line
	}
	if line >= m.agOffset+vis {
		m.agOffset = line - vis + 1
	}
	m.agOffset = max(0, min(m.agOffset, len(lines)-1))
}

func (m Model) handleAgentsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.agOpen != "" {
		switch k.String() {
		case "q":
			return m, tea.Quit
		case "esc", "enter", "h", "left":
			m.agOpen = ""
		case "?":
			m.showHelp = true
		}
		return m, nil
	}
	rows := m.agentList()
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.screen == screenProject {
			m.openSection(secProjects)
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
		return m, tea.Batch(m.startRefresh(), readAgents)
	case "j", "down":
		m.agCursor++
	case "k", "up":
		m.agCursor--
	case "g", "home":
		m.agCursor = 0
	case "G", "end":
		m.agCursor = 1 << 30
	case "enter", "l", "right":
		if m.agCursor >= 0 && m.agCursor < len(rows) {
			m.agOpen = rows[m.agCursor].ID
		}
	}
	m.agClamp()
	return m, nil
}
