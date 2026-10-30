package tui

import (
	"errors"
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

func agentTick() tea.Cmd {
	return tea.Tick(agentPoll, func(time.Time) tea.Msg { return agentTickMsg{} })
}

// agentRoot is the project a session belongs to, "" when it's in none you watch.
func (m Model) agentRoot(s agents.Session) string {
	if s.Project != "" {
		return s.Project
	}
	return m.watchedRoot(s.Dir)
}

// agentProject names a session's project: the watched one it's in, else its
// folder.
func (m Model) agentProject(s agents.Session) string {
	return filepath.Base(m.agentRoot(s))
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

// watchedAgents leaves out sessions in folders you don't watch here.
func (m Model) watchedAgents() []agents.Session {
	var out []agents.Session
	for _, s := range m.agents {
		if m.agentRoot(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// agentList is the sessions in view: every watched project's, or one's.
func (m Model) agentList() []agents.Session {
	list := m.watchedAgents()
	if m.screen != screenProject {
		return list
	}
	var out []agents.Session
	for _, s := range list {
		if m.agentRoot(s) == m.scope {
			out = append(out, s)
		}
	}
	return out
}

func agentAttention(m *Model) []attnItem {
	var out []attnItem
	for _, s := range m.watchedAgents() {
		if s.State != agents.Waiting {
			continue
		}
		out = append(out, attnItem{sev: attnWarn, text: s.Tool + " is waiting on you: " + agentTitle(s), at: s.Updated,
			project: m.agentRoot(s), to: target{section: secAgents}})
	}
	return out
}

func agentWinRows(m *Model) []winRow {
	var rows []winRow
	for _, s := range m.watchedAgents() {
		if s.Active() {
			rows = append(rows, winRow{text: agentDot(s.State) + " " + pad(m.agentProject(s), 14) + " " + pad(s.Tool, 9) + sDim.Render(agentTitle(s)), to: target{section: secAgents}})
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
		if m.agentRoot(s) != project {
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
		return []string{"", sDim.Render("No agents in your watched projects in the last week.")}, nil
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
	if s.Run != nil {
		lines = append(lines, row("Branch", s.Run.Branch))
		if s.Run.PR != "" {
			lines = append(lines, row("PR", s.Run.PR))
		}
		if s.Running {
			return append(lines, "", sDim.Render("Press a to open its terminal. ctrl+] comes back."))
		}
		lines = append(lines, row("Worktree", config.ShortHome(s.Run.Worktree)))
	}
	if s.ID == "" || strings.HasPrefix(s.ID, "oryn-") {
		return lines
	}
	return append(lines, "", sDim.Render("Resume it"), "  cd "+config.ShortHome(s.Dir)+" && "+s.Resume())
}

// agSelected is the session the detail page or the cursor is on.
func (m Model) agSelected() (agents.Session, bool) {
	rows := m.agentList()
	for i, s := range rows {
		if (m.agOpen != "" && s.ID == m.agOpen) || (m.agOpen == "" && i == m.agCursor) {
			return s, true
		}
	}
	return agents.Session{}, false
}

type attachedMsg struct{ err error }
type stoppedMsg struct{ text string }

func (m Model) attachAgent() (tea.Model, tea.Cmd) {
	s, ok := m.agSelected()
	switch {
	case !ok:
		return m, nil
	case s.Run == nil:
		m.flash = "oryn didn't start this one, so it can't attach. Resume it with: " + s.Resume()
		return m, nil
	}
	return m.attachRun(*s.Run)
}

func (m Model) attachRun(r agents.Run) (tea.Model, tea.Cmd) {
	return m, tea.ExecProcess(agents.AttachCmd(r), func(err error) tea.Msg {
		if err != nil {
			return attachedMsg{errors.New("couldn't attach: " + err.Error())}
		}
		return attachedMsg{}
	})
}

func (m Model) askStop() (tea.Model, tea.Cmd) {
	s, ok := m.agSelected()
	switch {
	case !ok:
	case s.Run == nil:
		m.flash = "oryn didn't start this one, so it won't stop it"
	case !s.Running:
		m.flash = "already stopped"
	default:
		m.stopAsk = s.Run
	}
	return m, nil
}

func (m Model) handleStopConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := *m.stopAsk
	m.stopAsk = nil
	if k.String() != "y" {
		m.flash = "cancelled, still running"
		return m, nil
	}
	return m, func() tea.Msg {
		if err := agents.Stop(r); err != nil {
			return stoppedMsg{"couldn't stop " + r.Slug + ": " + err.Error()}
		}
		return stoppedMsg{"stopped " + r.Slug + ", its work is on " + r.Branch}
	}
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
		case "a":
			return m.attachAgent()
		case "x":
			return m.askStop()
		case "v":
			return m.openReview()
		case "P":
			return m.askShip()
		case "X":
			return m.askDiscard()
		}
		return m, nil
	}
	rows := m.agentList()
	switch k.String() {
	case "n":
		return m.openSpawn()
	case "a":
		return m.attachAgent()
	case "x":
		return m.askStop()
	case "v":
		return m.openReview()
	case "P":
		return m.askShip()
	case "X":
		return m.askDiscard()
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
