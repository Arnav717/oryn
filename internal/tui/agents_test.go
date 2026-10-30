package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/agents"
)

func agentModel() Model {
	m := busyModel()
	t := m.now
	m.agents = []agents.Session{
		{Tool: "claude", ID: "w", Dir: "/code/a/sub", Title: "ship login", State: agents.Waiting, Needs: "approve deploy", Updated: t.Add(-time.Hour)},
		{Tool: "agy", ID: "r", Dir: "/code/b", Title: "fix tests", State: agents.Working, Updated: t},
		{Tool: "codex", ID: "x", Dir: "/elsewhere/tool", Title: "outside", State: agents.Waiting, Updated: t},
		{Tool: "opencode", ID: "i", Dir: "/code/a", State: agents.Idle, Updated: t.Add(-2 * time.Hour)},
	}
	return m
}

func TestAgentWaitingReachesNeedsYou(t *testing.T) {
	m := agentModel()
	items := m.needsYou()
	got := itemTexts(items)
	if !strings.Contains(got, "claude is waiting on you: ship login") {
		t.Fatalf("queue:\n%s", got)
	}
	if strings.Contains(got, "fix tests") {
		t.Errorf("a running agent doesn't need you:\n%s", got)
	}
	// Inside a watched project it counts for that project, outside it keeps its folder name.
	if !strings.Contains(itemTexts(m.needsFor("/code/a")), "ship login") {
		t.Errorf("project a: %s", itemTexts(m.needsFor("/code/a")))
	}
	if out := m.View(); !strings.Contains(out, "tool") {
		t.Errorf("unwatched folder name missing:\n%s", out)
	}
}

func TestAgentWindowShowsOnlyActive(t *testing.T) {
	m := agentModel()
	var text []string
	for _, r := range agentWinRows(&m) {
		text = append(text, r.text)
	}
	out := strings.Join(text, "\n")
	if !strings.Contains(out, "ship login") || !strings.Contains(out, "fix tests") || strings.Contains(out, "untitled") {
		t.Errorf("window:\n%s", out)
	}
	m.agents = nil
	if rows := agentWinRows(&m); len(rows) != 1 || !strings.Contains(rows[0].text, "none running") {
		t.Errorf("empty: %+v", rows)
	}
}

func TestAgentsTab(t *testing.T) {
	m := press(t, agentModel(), "3")
	out := strings.Join(m.agentBody(), "\n")
	for _, want := range []string{"Waiting on you", "Running", "Recent", "ship login", "fix tests", "untitled"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}

	m = press(t, m, "enter")
	out = strings.Join(m.agentBody(), "\n")
	for _, want := range []string{"approve deploy", "claude --resume w", "/code/a/sub"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
	m = press(t, m, "esc")
	if m.agOpen != "" || m.screen != screenAgents {
		t.Errorf("esc from detail: %q screen %d", m.agOpen, m.screen)
	}
	if got := press(t, m, "esc"); got.screen != screenHome {
		t.Errorf("esc from list: %d", got.screen)
	}
}

func TestAgentCursorSurvivesAShrinkingList(t *testing.T) {
	m := press(t, agentModel(), "3", "G")
	next, _ := m.Update(agentsMsg{m.agents[:1]})
	if got := next.(Model); got.agCursor != 0 {
		t.Errorf("cursor %d", got.agCursor)
	}
	// Detail on a session that ended falls back to the list.
	m = press(t, agentModel(), "3", "enter")
	next, _ = m.Update(agentsMsg{nil})
	if out := strings.Join(next.(Model).agentBody(), "\n"); !strings.Contains(out, "No agent sessions") {
		t.Errorf("view:\n%s", out)
	}
}

func TestProjectAgentsTabIsScoped(t *testing.T) {
	m := agentModel()
	m.openScope("/code/a")
	m.setTab(tabAgents)
	out := strings.Join(m.agentBody(), "\n")
	if !strings.Contains(out, "ship login") || strings.Contains(out, "fix tests") || strings.Contains(out, "outside") {
		t.Errorf("scoped:\n%s", out)
	}
	if got := press(t, m, "esc"); got.screen != screenProjects {
		t.Errorf("esc: %d", got.screen)
	}
}

func TestOverviewAgentsRow(t *testing.T) {
	m := agentModel()
	m.openScope("/code/a")
	if out := m.View(); !strings.Contains(out, "1 waiting") {
		t.Errorf("overview:\n%s", out)
	}
	m.openScope("/code/b")
	if out := m.View(); !strings.Contains(out, "1 running") {
		t.Errorf("overview b:\n%s", out)
	}
}

func TestAgentsScreenFits(t *testing.T) {
	m := agentModel()
	for range 60 {
		m.agents = append(m.agents, agents.Session{Tool: "claude", ID: "z", Dir: "/code/a", Title: strings.Repeat("long title ", 30), State: agents.Idle})
	}
	for _, sz := range [][2]int{{60, 14}, {120, 30}} {
		m.w, m.h = sz[0], sz[1]
		got := press(t, m, "3")
		for range 70 {
			got = press(t, got, "j")
		}
		lines := strings.Split(got.View(), "\n")
		if len(lines) > got.h {
			t.Errorf("%dx%d: %d lines", got.w, got.h, len(lines))
		}
		_, rowLine := got.agentLines()
		vis := got.h - got.chrome() - 2
		if l := rowLine[got.agCursor]; l < got.agOffset || l >= got.agOffset+vis {
			t.Errorf("cursor line %d outside [%d,%d)", l, got.agOffset, got.agOffset+vis)
		}
	}
}
