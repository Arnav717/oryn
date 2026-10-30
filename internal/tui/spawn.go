package tui

import (
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/agents"
)

type spawnStep int

const (
	spawnProject spawnStep = iota
	spawnTool
	spawnTask
)

// spawnForm is the "new agent" flow: project, tool, task.
type spawnForm struct {
	step    spawnStep
	project string
	tool    string
	tools   []string
	cursor  int
	input   textinput.Model
	dirty   bool
	busy    bool
	prefill string
}

func (f spawnForm) hints() string {
	if f.step == spawnTask {
		return "enter start · esc back"
	}
	return "enter choose · esc back"
}

type spawnedMsg struct {
	run agents.Run
	err error
}

func (m Model) openSpawn() (tea.Model, tea.Cmd) {
	tools := agents.InstalledTools()
	if len(tools) == 0 {
		m.flash = "no agent tools found: install claude, codex, opencode or agy"
		return m, nil
	}
	if len(m.cfg.Projects) == 0 {
		m.flash = "add a project first"
		return m, nil
	}
	f := &spawnForm{tools: tools}
	if m.screen == screenProject {
		f.project = m.scope
	} else if len(m.cfg.Projects) == 1 {
		f.project = m.cfg.Projects[0]
	}
	if f.project != "" {
		f.step = spawnTool
	}
	m.spawn = f
	return m, f.advanceIfOneTool()
}

// openSpawnWith starts the form on a project with the task already written.
func (m Model) openSpawnWith(project, task string) (tea.Model, tea.Cmd) {
	next, _ := m.openSpawn()
	m = next.(Model)
	if m.spawn == nil {
		return m, nil
	}
	f := &spawnForm{tools: m.spawn.tools, project: project, step: spawnTool, prefill: task}
	m.spawn = f
	return m, f.advanceIfOneTool()
}

// advanceIfOneTool skips the tool question when there's nothing to choose.
func (f *spawnForm) advanceIfOneTool() tea.Cmd {
	if f.step == spawnTool && len(f.tools) == 1 {
		f.tool = f.tools[0]
		return f.toTask()
	}
	return nil
}

func (f *spawnForm) toTask() tea.Cmd {
	f.step = spawnTask
	f.dirty = agents.Dirty(f.project)
	f.input = textinput.New()
	f.input.Prompt = ""
	f.input.Placeholder = "What should it do?"
	f.input.CharLimit = 2000
	f.input.SetValue(f.prefill)
	return f.input.Focus()
}

func (m Model) handleSpawnKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := *m.spawn
	if f.busy {
		return m, nil
	}
	list := f.tools
	if f.step == spawnProject {
		list = m.cfg.Projects
	}
	switch k.Type {
	case tea.KeyEsc:
		switch {
		case f.step == spawnTask && len(f.tools) > 1:
			f.step, f.cursor = spawnTool, 0
		case f.step == spawnTool && m.screen != screenProject && len(m.cfg.Projects) > 1:
			f.step, f.cursor = spawnProject, 0
		default:
			m.spawn = nil
			return m, nil
		}
		m.spawn = &f
		return m, nil
	case tea.KeyEnter:
		switch f.step {
		case spawnProject:
			f.project, f.step, f.cursor = list[f.cursor], spawnTool, 0
			cmd := f.advanceIfOneTool()
			m.spawn = &f
			return m, cmd
		case spawnTool:
			f.tool = list[f.cursor]
			cmd := f.toTask()
			m.spawn = &f
			return m, cmd
		}
		task := f.input.Value()
		f.busy = true
		m.spawn = &f
		project, tool := f.project, f.tool
		return m, func() tea.Msg {
			r, err := agents.Start(project, tool, task, time.Now())
			return spawnedMsg{r, err}
		}
	}
	if f.step == spawnTask {
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(k)
		m.spawn = &f
		return m, cmd
	}
	switch k.String() {
	case "j", "down":
		f.cursor = min(f.cursor+1, len(list)-1)
	case "k", "up":
		f.cursor = max(f.cursor-1, 0)
	}
	m.spawn = &f
	return m, nil
}

// onSpawned keeps the form open on failure, so a long task isn't lost.
func (m Model) onSpawned(msg spawnedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if m.spawn != nil {
			f := *m.spawn
			f.busy = false
			m.spawn = &f
		}
		m.flash = msg.err.Error()
		return m, nil
	}
	m.spawn = nil
	if m.screen != screenProject {
		m.openSection(secAgents)
	} else {
		m.setTab(tabAgents)
	}
	m.flash = "started " + msg.run.Tool + " on " + msg.run.Branch + " · a attaches"
	return m, readAgents
}

func (m Model) spawnLines() []string {
	f := m.spawn
	lines := []string{sTitle.Render("New agent"), ""}
	row := func(i int, label string) string {
		if i == f.cursor {
			return sAccent.Render("▸ ") + label
		}
		return "  " + label
	}
	switch f.step {
	case spawnProject:
		lines = append(lines, sDim.Render("Project"))
		for i, p := range m.cfg.Projects {
			lines = append(lines, row(i, filepath.Base(p)))
		}
	case spawnTool:
		lines = append(lines, sDim.Render(filepath.Base(f.project)), "", sDim.Render("Tool"))
		for i, t := range f.tools {
			lines = append(lines, row(i, t))
		}
	case spawnTask:
		lines = append(lines, sDim.Render(filepath.Base(f.project)+" · "+f.tool), "", f.input.View(), "",
			sDim.Render("Runs in its own worktree and branch, from your last commit."),
			sDim.Render(".env and other gitignored files aren't copied in."))
		if f.dirty {
			lines = append(lines, sWarn.Render("You have uncommitted changes. This agent won't see them."))
		}
		if f.busy {
			lines = append(lines, "", sAccent.Render("starting…"))
		}
	}
	return lines
}
