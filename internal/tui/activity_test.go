package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
)

func activityModel() Model {
	m := busyModel()
	now := m.now
	m.since = now.Add(-time.Hour)
	m.snap.Events = []store.Event{
		{Key: "d1", Kind: "deploy", Severity: 4, Entity: "/code/b", Title: "vercel deploy failed", At: now.Add(-time.Minute)},
		{Key: "s1", Kind: "status", Severity: 2, Entity: "Cloudflare", Title: "Cloudflare: outage", At: now.Add(-2 * time.Minute)},
		{Key: "p1", Kind: "pr", Severity: 1, Entity: "o/r", Title: "new PR on o/r", At: now.Add(-3 * time.Hour)},
		{Key: "repo:o/r", Kind: "repo", Entity: "o/r", Title: "baseline", At: now.Add(-4 * time.Hour)},
	}
	return m
}

func TestActivitySplitsNewFromEarlier(t *testing.T) {
	m := press(t, activityModel(), "7")
	out := strings.Join(mustLines(m), "\n")
	n, e := strings.Index(out, "New since"), strings.Index(out, "Earlier")
	if n < 0 || e < n {
		t.Fatalf("sections:\n%s", out)
	}
	if !strings.Contains(out[n:e], "vercel deploy failed") || !strings.Contains(out[e:], "new PR on o/r") {
		t.Errorf("wrong section:\n%s", out)
	}
	if strings.Contains(out, "baseline") {
		t.Errorf("repo baselines are bookkeeping:\n%s", out)
	}
}

func TestActivityWithNothingNewStillShowsTheTimeline(t *testing.T) {
	m := activityModel()
	m.since = m.now
	m = press(t, m, "7")
	out := strings.Join(mustLines(m), "\n")
	if strings.Contains(out, "New since") || !strings.Contains(out, "new PR on o/r") {
		t.Errorf("timeline:\n%s", out)
	}
}

func TestActivityFilter(t *testing.T) {
	m := press(t, activityModel(), "7", "f") // deploys
	out := strings.Join(mustLines(m), "\n")
	if !strings.Contains(out, "vercel deploy failed") || strings.Contains(out, "new PR") || strings.Contains(out, "outage") {
		t.Errorf("deploys filter:\n%s", out)
	}
	m = press(t, m, "f") // github
	out = strings.Join(mustLines(m), "\n")
	if strings.Contains(out, "vercel deploy failed") || !strings.Contains(out, "new PR") {
		t.Errorf("github filter:\n%s", out)
	}
	if got := press(t, m, "f", "f"); got.chFilter != 0 {
		t.Errorf("filter should wrap, got %d", got.chFilter)
	}
}

func TestProjectActivityIsScoped(t *testing.T) {
	m := activityModel()
	m.openScope("/code/a")
	m.setTab(tabActivity)
	out := strings.Join(mustLines(m), "\n")
	if !strings.Contains(out, "new PR on o/r") {
		t.Errorf("own repo event missing:\n%s", out)
	}
	if strings.Contains(out, "vercel deploy failed") || strings.Contains(out, "outage") {
		t.Errorf("other project's or upstream event leaked:\n%s", out)
	}

	m.openScope("/code/b")
	m.setTab(tabActivity)
	if out := strings.Join(mustLines(m), "\n"); !strings.Contains(out, "vercel deploy failed") || strings.Contains(out, "new PR") {
		t.Errorf("b:\n%s", out)
	}
}

func TestActivityScrollKeepsCursorVisible(t *testing.T) {
	m := activityModel()
	m.h = 16
	m.snap.Events = nil
	for i := range 40 {
		m.snap.Events = append(m.snap.Events, store.Event{Key: string(rune('a' + i)), Kind: "pr", Entity: "o/r", Title: "ev", At: m.now.Add(-2 * time.Hour)})
	}
	m = press(t, m, "7")
	vis := m.h - m.chrome() - 2
	for range 39 {
		m = press(t, m, "j")
		_, _, rowLine := m.activity()
		if l := rowLine[m.chCursor]; l < m.chOffset || l >= m.chOffset+vis {
			t.Fatalf("cursor line %d outside [%d,%d)", l, m.chOffset, m.chOffset+vis)
		}
	}
}

func TestCommandsReachHealthAndActivity(t *testing.T) {
	m := shellModel()
	got, _ := m.runCommand("v")
	if g := got.(Model); g.screen != screenHealth || g.focus != pVulns {
		t.Errorf("v from home: screen %d focus %d", g.screen, g.focus)
	}
	m.openScope("/code/a")
	got, _ = m.runCommand("tape")
	if g := got.(Model); g.tab == tabActivity {
		t.Errorf("tape should no longer be a command")
	}
	got, _ = m.runCommand("activity")
	if g := got.(Model); g.tab != tabActivity {
		t.Errorf("activity: tab %d", g.tab)
	}
	got, _ = m.runCommand("s")
	if g := got.(Model); g.tab != tabHealth || g.focus != pStatus {
		t.Errorf("s in project: tab %d focus %d", g.tab, g.focus)
	}
}

func TestProjectTabBarKeepsEveryTab(t *testing.T) {
	m := busyModel()
	m.openScope("/code/a")
	for _, w := range []int{60, 80, 120} {
		m.w = w
		line := m.tabBarView()
		for _, name := range tabNames {
			if !strings.Contains(line, name) {
				t.Errorf("width %d lost tab %s: %q", w, name, line)
			}
		}
	}
}

func mustLines(m Model) []string {
	lines, _, _ := m.activity()
	return lines
}

func TestUpstreamPaneListsProblemsFirst(t *testing.T) {
	m := busyModel()
	m.snap.Status = []source.Status{
		{Service: "GitHub", Indicator: "none", Description: "ok"},
		{Service: "Cloudflare", Indicator: "major", Description: "Outage"},
	}
	_, rows := m.statusRows(80)
	if !strings.Contains(rows[0], "Cloudflare") {
		t.Errorf("rows: %q", rows)
	}
}
