package tui

import (
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/store"
)

func TestSinceEvents(t *testing.T) {
	now := time.Now()
	m := Model{
		since: now.Add(-2 * time.Hour),
		snap: &app.Snapshot{Events: []store.Event{
			{Key: "a", Kind: "vuln", Severity: 4, At: now.Add(-time.Hour)},
			{Key: "b", Kind: "release", At: now.Add(-3 * time.Hour)},
			{Key: "c", Kind: "repo", At: now.Add(-time.Minute)},
			{Key: "d", Kind: "status", Severity: 2, At: now.Add(-time.Minute)},
		}},
	}
	got := m.sinceEvents()
	if len(got) != 2 || got[0].Key != "a" || got[1].Key != "d" {
		t.Fatalf("got %+v", got)
	}
	m.since = time.Time{}
	if m.sinceEvents() != nil {
		t.Fatal("first run must show nothing")
	}
}
