package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/deploy"
)

func TestDeployLines(t *testing.T) {
	now := time.Now()
	m := Model{now: now, snap: &app.Snapshot{Deploys: []app.DeployTarget{
		{Link: deploy.Link{Platform: "vercel", Project: "/p/web", Ref: "prj", Marker: ".vercel/project.json"}, Queryable: true,
			Deployments: []deploy.Deployment{
				{ID: "2", Target: "preview", State: deploy.StateBuilding, Created: now},
				{ID: "1", Target: "production", State: deploy.StateError, Branch: "main", Commit: "abc1234", Created: now.Add(-time.Hour)},
			}},
		{Link: deploy.Link{Platform: "netlify", Project: "/p/docs", Marker: "netlify.toml"}},
	}}}
	out := strings.Join(m.deployLines(), "\n")
	for _, want := range []string{"DEPLOYS", "web", "error", "main", "abc1234", "not linked"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if m.deploySummary() == "" || !strings.Contains(m.deploySummary(), "1 failing") {
		t.Errorf("summary: %q", m.deploySummary())
	}
	if (Model{}).deployLines() != nil {
		t.Error("no snapshot should render nothing")
	}
}
