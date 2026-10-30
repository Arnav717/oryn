package tui

import (
	"strings"
	"testing"

	"github.com/Arnav717/oryn/internal/agents"
)

func runModel() Model {
	m := agentModel()
	r := &agents.Run{Slug: "x", Branch: "oryn/x", Task: "do it", Project: "/code/a", Worktree: "/w/x"}
	m.agents[0].Run, m.agents[0].Running = r, true
	m.screen = screenAgents
	return m
}

func TestReviewOnlyOwnAgents(t *testing.T) {
	m := press(t, agentModel(), "3")
	for key, want := range map[string]string{"v": "no worktree", "P": "won't push", "X": "won't delete"} {
		got := press(t, m, key)
		if got.review != nil || got.shipAsk != nil || got.discard != nil || !strings.Contains(got.flash, want) {
			t.Errorf("%s: %q", key, got.flash)
		}
	}
}

func TestReviewShowsDiff(t *testing.T) {
	m := runModel()
	m.review = &reviewView{run: *m.agents[0].Run, loading: true}
	next, _ := m.Update(reviewMsg{"x", agents.Changes{
		Diff: "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-old\n+new", Untracked: []string{"fresh.txt"}, Commits: 2, Dirty: 1,
	}, nil})
	got := next.(Model)
	out := got.View()
	for _, want := range []string{"2 commit(s)", "1 uncommitted", "new file  fresh.txt", "-old", "+new", "oryn/x"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--- a/a") {
		t.Errorf("header noise:\n%s", out)
	}
	if got := press(t, got, "esc"); got.review != nil {
		t.Error("esc should close the review")
	}
}

func TestDiffCannotDriveTheTerminal(t *testing.T) {
	lines := diffLines(agents.Changes{Diff: "+\x1b[2Jevil\x1b]0;title\x07\r\n+\ttab", Untracked: []string{"a\x1b[31mb"}})
	for _, l := range lines {
		if strings.ContainsAny(l, "\x07\r") || strings.Contains(l, "[2J") || strings.Contains(l, "\x1b]") {
			t.Errorf("unsafe: %q", l)
		}
	}
}

func TestReviewScrollStops(t *testing.T) {
	m := runModel()
	m.review = &reviewView{run: *m.agents[0].Run, lines: make([]string, 100)}
	got := press(t, m, "G", "j", "j")
	if lim := 100 - max(1, m.h-m.chrome()-4); got.review.offset != lim {
		t.Errorf("offset %d, want %d", got.review.offset, lim)
	}
	if got := press(t, m, "k"); got.review.offset != 0 {
		t.Errorf("offset %d", got.review.offset)
	}
}

func TestShipAsksFirst(t *testing.T) {
	m := runModel()
	m = press(t, m, "P")
	if m.shipAsk == nil || !strings.Contains(m.View(), "open a PR") {
		t.Fatalf("no prompt:\n%s", m.View())
	}
	if got := press(t, m, "n"); got.shipAsk != nil || !strings.Contains(got.flash, "nothing pushed") {
		t.Errorf("cancel: %q", got.flash)
	}
	next, cmd := m.handleKey(keyMsg("y"))
	if next.(Model).shipAsk != nil || cmd == nil {
		t.Error("y should ship")
	}
}

func TestShipResult(t *testing.T) {
	m := runModel()
	next, _ := m.Update(shippedMsg{err: errBoom{}})
	if !strings.Contains(next.(Model).flash, "boom") {
		t.Errorf("flash: %q", next.(Model).flash)
	}
	next, _ = m.Update(shippedMsg{url: "https://x/pull/1"})
	if !strings.Contains(next.(Model).flash, "pull/1") {
		t.Errorf("flash: %q", next.(Model).flash)
	}
}

func TestDiscardNeedsTheExactName(t *testing.T) {
	m := press(t, runModel(), "X")
	if m.discard == nil {
		t.Fatal("no prompt")
	}
	wrong := press(t, typed(t, m, "nope"), "enter")
	if wrong.discard != nil || !strings.Contains(wrong.flash, "nothing deleted") {
		t.Errorf("wrong name: %q", wrong.flash)
	}
	if got := press(t, m, "esc"); got.discard != nil {
		t.Error("esc should cancel")
	}
	m = typed(t, m, "x")
	next, cmd := m.handleKey(keyMsg("enter"))
	if next.(Model).discard != nil || cmd == nil {
		t.Error("the right name should discard")
	}
}

func TestFixStartsAnAgentWithTheTask(t *testing.T) {
	m := busyModel()
	rows := needsRows(&m)
	if rows[0].fix == "" {
		t.Fatalf("first row has no fix: %+v", rows[0])
	}
	got := press(t, m, "f")
	if got.spawn == nil {
		t.Skip("no agent tools on PATH")
	}
	if got.spawn.prefill != rows[0].fix || got.spawn.project != rows[0].to.project {
		t.Errorf("form: %+v", got.spawn)
	}
}

func TestFixPromptsCarryNoFetchedText(t *testing.T) {
	m := busyModel()
	m.snap.Repos["/code/a"].Runs = nil
	for _, it := range m.needsYou() {
		if strings.Contains(it.fix, "IGNORE") {
			t.Errorf("fetched text in a task: %q", it.fix)
		}
	}
	got := press(t, m, "j", "j", "f") // a review row has nothing to fix
	if got.spawn != nil {
		t.Errorf("form opened: %+v", got.spawn)
	}
}
