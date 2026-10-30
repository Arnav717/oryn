package tui

import (
	"testing"

	"github.com/Arnav717/oryn/internal/deploy"
)

func TestClicksWaitForPendingQuestions(t *testing.T) {
	m := busyModel()
	m.openScope("/code/a")
	r := m.gridLayout()[1]
	m.deployAsk = &deploy.Command{}
	if got := m.handleClick(r.x+2, r.y+m.chrome()); got.tab != tabOverview {
		t.Error("a click must not move past a deploy prompt")
	}
}
