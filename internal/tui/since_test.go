package tui

import (
	"strings"
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

func TestGroupSince(t *testing.T) {
	evs := []store.Event{
		{Key: "vuln:A:PyPI:pillow@12.3.0", Kind: "vuln", Severity: 4},
		{Key: "vuln:B:PyPI:pillow@12.3.0", Kind: "vuln", Severity: 3},
		{Key: "vuln:C:npm:@sentry/node@1.0.0", Kind: "vuln", Severity: 3},
		{Key: "review:x", Kind: "review", Severity: 2},
	}
	rows := groupSince(evs)
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if !strings.Contains(rows[0].line, "2 vulns in pillow@12.3.0, worst critical") {
		t.Errorf("group line: %q", rows[0].line)
	}
}
