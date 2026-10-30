package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/deploy"
)

func TestNeedsYouSetAsideAndBack(t *testing.T) {
	m := busyModel()
	needs := func(m Model) string { return itemTexts(m.needsYou()) }
	if !strings.Contains(needs(m), "vercel deploy failed") {
		t.Fatalf("start: %s", needs(m))
	}
	rows := needsRows(&m)
	cur := 0
	for i, r := range rows {
		if strings.Contains(r.text, "vercel deploy failed") {
			cur = i
		}
	}
	m.winCur[winNeeds] = cur
	hidden := press(t, m, "x")
	if strings.Contains(needs(hidden), "vercel deploy failed") {
		t.Errorf("dismissed but still listed: %s", needs(hidden))
	}
	if last := needsRows(&hidden); !strings.Contains(last[len(last)-1].text, "1 set aside") {
		t.Errorf("should say something's set aside: %+v", last)
	}

	// A new failure under the same heading isn't the one you dismissed.
	again := hidden
	dep := &again.snap.Deploys[0].Deployments[0]
	saved := dep.Created
	dep.Created = saved.Add(time.Minute)
	if !strings.Contains(needs(again), "vercel deploy failed") {
		t.Error("a new failure should show up again")
	}
	dep.Created = saved

	if got := press(t, hidden, "u"); !strings.Contains(needs(got), "vercel deploy failed") {
		t.Errorf("u should bring it back: %s", needs(got))
	}

	m.hidden = nil
	snoozed := press(t, m, "z")
	snoozed.now = snoozed.now.Add(snoozeFor + time.Minute)
	if !strings.Contains(needs(snoozed), "vercel deploy failed") {
		t.Error("a snooze should run out")
	}
}

func TestSetAsideOnAProjectsWindows(t *testing.T) {
	m := busyModel()
	m.openScope("/code/a")
	got := press(t, m, "x")
	if strings.Contains(itemTexts(got.needsFor("/code/a")), "CI is failing") {
		t.Error("x should dismiss the selected item")
	}
	if got := press(t, got, "u"); !strings.Contains(itemTexts(got.needsFor("/code/a")), "CI is failing") {
		t.Error("u should bring it back")
	}
	m.pwin = pwAgents
	if got := press(t, m, "x"); !strings.Contains(got.flash, "nothing here") {
		t.Errorf("flash %q", got.flash)
	}
}

func TestClicksWaitForPendingQuestions(t *testing.T) {
	m := busyModel()
	m.openScope("/code/a")
	r := m.gridLayout()[1]
	m.deployAsk = &deploy.Command{}
	if got := m.handleClick(r.x+2, r.y+m.chrome()); got.tab != tabOverview {
		t.Error("a click must not move past a deploy prompt")
	}
}
