package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/agents"
)

func formModel(project string, tools ...string) Model {
	m := busyModel()
	f := &spawnForm{tools: tools}
	if project != "" {
		f.project, f.step = project, spawnTool
	}
	m.spawn = f
	return m
}

func TestSpawnFormSteps(t *testing.T) {
	m := formModel("", "claude", "agy")
	if !strings.Contains(m.View(), "New agent") {
		t.Fatalf("view:\n%s", m.View())
	}
	m = press(t, m, "j", "enter") // second project
	if m.spawn.project != "/code/b" || m.spawn.step != spawnTool {
		t.Fatalf("after project: %+v", m.spawn)
	}
	m = press(t, m, "j", "enter") // agy
	if m.spawn.tool != "agy" || m.spawn.step != spawnTask {
		t.Fatalf("after tool: %+v", m.spawn)
	}
	m = typed(t, m, "12 go")
	if m.spawn.input.Value() != "12 go" || m.screen != screenHome {
		t.Errorf("number keys must type, not switch tabs: %q screen %d", m.spawn.input.Value(), m.screen)
	}
	m = press(t, m, "esc")
	if m.spawn.step != spawnTool {
		t.Errorf("esc from task: %+v", m.spawn)
	}
	m = press(t, m, "esc", "esc")
	if m.spawn != nil {
		t.Error("esc should close the form")
	}
}

func TestSpawnFailureKeepsTheTask(t *testing.T) {
	m := formModel("/code/a", "claude", "agy")
	m = press(t, m, "enter")
	m = typed(t, m, "long careful task")
	m.spawn.busy = true
	next, _ := m.Update(spawnedMsg{err: errBoom{}})
	got := next.(Model)
	if got.spawn == nil || got.spawn.busy || got.spawn.input.Value() != "long careful task" || !strings.Contains(got.flash, "boom") {
		t.Errorf("after failure: %+v flash %q", got.spawn, got.flash)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }

func TestSpawnSuccessShowsAgents(t *testing.T) {
	m := formModel("/code/a", "claude")
	next, _ := m.Update(spawnedMsg{run: agents.Run{Tool: "claude", Branch: "oryn/x"}})
	got := next.(Model)
	if got.spawn != nil || got.screen != screenAgents || !strings.Contains(got.flash, "oryn/x") {
		t.Errorf("after start: spawn %v screen %d flash %q", got.spawn, got.screen, got.flash)
	}
}

func TestSpawnWarnsAboutUncommittedWork(t *testing.T) {
	m := formModel("/code/a", "claude")
	m.spawn.step, m.spawn.dirty = spawnTask, true
	if out := strings.Join(m.spawnLines(), "\n"); !strings.Contains(out, "uncommitted") {
		t.Errorf("lines:\n%s", out)
	}
}

func TestAttachAndStopOnlyOwnAgents(t *testing.T) {
	m := press(t, agentModel(), "3") // first row wasn't started by oryn
	got := press(t, m, "a")
	if !strings.Contains(got.flash, "claude --resume w") {
		t.Errorf("attach foreign: %q", got.flash)
	}
	got = press(t, m, "x")
	if got.stopAsk != nil || !strings.Contains(got.flash, "won't stop") {
		t.Errorf("stop foreign: %+v %q", got.stopAsk, got.flash)
	}
}

func TestStopAsksFirst(t *testing.T) {
	m := agentModel()
	r := &agents.Run{Slug: "x", Branch: "oryn/x"}
	m.agents[0].Run, m.agents[0].Running = r, true
	m = press(t, m, "3", "x")
	if m.stopAsk == nil || !strings.Contains(m.View(), "oryn/x") {
		t.Fatalf("no prompt:\n%s", m.View())
	}
	if got := press(t, m, "n"); got.stopAsk != nil || !strings.Contains(got.flash, "still running") {
		t.Errorf("cancel: %+v %q", got.stopAsk, got.flash)
	}
	next, cmd := m.handleKey(keyMsg("y"))
	if next.(Model).stopAsk != nil || cmd == nil {
		t.Error("y should stop it")
	}
	m.agents[0].Running = false
	m.stopAsk = nil
	if got := press(t, m, "x"); got.stopAsk != nil || got.flash != "already stopped" {
		t.Errorf("stopped: %+v %q", got.stopAsk, got.flash)
	}
}

func TestOrynAgentCountsForItsProject(t *testing.T) {
	m := agentModel()
	m.agents = []agents.Session{{Tool: "claude", ID: "z", Dir: "/data/oryn-state/worktrees/a/x", Project: "/code/a", Title: "in worktree", State: agents.Working}}
	m.openScope("/code/a")
	m.setTab(tabAgents)
	if out := strings.Join(m.agentBody(), "\n"); !strings.Contains(out, "in worktree") {
		t.Errorf("worktree agent missing from its project:\n%s", out)
	}
}

func TestAgentPollingIsOneLoop(t *testing.T) {
	m := agentModel()
	if _, cmd := m.Update(agentsMsg{}); cmd != nil {
		t.Error("a result must not start another polling loop")
	}
	if _, cmd := m.Update(agentTickMsg{}); cmd == nil {
		t.Error("the tick keeps polling")
	}
	var _ tea.Msg = agentTickMsg{}
}
