package main

import (
	"strings"
	"testing"

	"github.com/Arnav717/oryn/internal/store"
)

func TestAlertWorthy(t *testing.T) {
	in := []store.Event{
		{Kind: "vuln", Severity: 4, Title: "crit"},
		{Kind: "vuln", Severity: 3, Title: "high"},
		{Kind: "vuln", Severity: 2, Title: "medium"},
		{Kind: "release", Title: "new release"},
		{Kind: "status", Severity: 0, Title: "recovered"},
		{Kind: "review", Severity: 2, Title: "review me"},
		{Kind: "ci", Severity: 3, Title: "ci red"},
	}
	got := alertWorthy(in)
	if len(got) != 5 || got[0].Title != "crit" || got[1].Title != "high" || got[2].Title != "recovered" || got[3].Title != "review me" || got[4].Title != "ci red" {
		t.Fatalf("got %+v", got)
	}
}

func TestNotifications(t *testing.T) {
	ev := func(n int) []store.Event {
		out := make([]store.Event, n)
		for i := range out {
			out[i] = store.Event{Kind: "vuln", Title: strings.Repeat("x", i+1)}
		}
		return out
	}
	if got := notifications(nil); len(got) != 0 {
		t.Errorf("no alerts should send nothing, got %v", got)
	}
	if got := notifications(ev(3)); len(got) != 3 {
		t.Errorf("3 alerts should send 3 notifications, got %d", len(got))
	}
	got := notifications(ev(7))
	if len(got) != 1 || got[0][0] != "oryn: 7 new alerts" || !strings.Contains(got[0][1], "and 4 more") {
		t.Errorf("7 alerts should collapse into one summary, got %v", got)
	}
}
