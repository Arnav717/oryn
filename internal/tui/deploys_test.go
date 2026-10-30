package tui

import (
	"github.com/Arnav717/oryn/internal/config"
	tea "github.com/charmbracelet/bubbletea"
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

func TestAccountsTokenEntry(t *testing.T) {
	m := Model{screen: screenAccounts}
	next, _ := m.handleAccountsKey(keyMsg("enter"))
	m = next.(Model)
	if !m.tokenEntry {
		t.Fatal("enter should open token entry")
	}
	next, _ = m.handleAccountsKey(keyMsg("q")) // typed into the field, must not quit
	m = next.(Model)
	if !m.tokenEntry || m.tokenInput.Value() != "q" {
		t.Fatalf("typing into the token field: entry=%v value=%q", m.tokenEntry, m.tokenInput.Value())
	}
	if strings.Contains(m.tokenInput.View(), "q") && !strings.Contains(m.tokenInput.View(), "•") {
		t.Error("token must be masked")
	}
	next, _ = m.handleAccountsKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.tokenEntry || m.tokenInput.Value() != "" {
		t.Error("esc should cancel and clear the token")
	}
}

func TestPickerLinksProject(t *testing.T) {
	cfg := &config.Config{Ephemeral: true}
	m := Model{
		cfg: cfg, screen: screenPicker, pickBack: screenDeploys, pickCursor: 1,
		pickTarget: app.DeployTarget{Link: deploy.Link{Platform: "vercel", Project: "/p/web"}},
		pickItems:  []deploy.ProjectRef{{Name: "a", Ref: "prj_a"}, {Name: "b", Ref: "prj_b", Team: "team_1"}},
	}
	next, _ := m.handlePickerKey(keyMsg("enter"))
	m = next.(Model)
	if m.screen != screenDeploys || len(cfg.DeployLinks) != 1 || cfg.DeployLinks[0].Ref != "prj_b" || cfg.DeployLinks[0].Team != "team_1" {
		t.Fatalf("screen=%v links=%+v", m.screen, cfg.DeployLinks)
	}
}

func TestDeployRowsScopedToProject(t *testing.T) {
	snap := &app.Snapshot{Deploys: []app.DeployTarget{
		{Link: deploy.Link{Platform: "vercel", Project: "/p/web", Ref: "a"}, Queryable: true,
			Deployments: []deploy.Deployment{{ID: "1", Target: "production", State: deploy.StateReady}}},
		{Link: deploy.Link{Platform: "netlify", Project: "/p/docs", Ref: "b"}, Queryable: true,
			Deployments: []deploy.Deployment{{ID: "2", Target: "production", State: deploy.StateReady}, {ID: "3", Target: "preview", State: deploy.StateReady}}},
	}}
	central := Model{snap: snap, screen: screenDeploys}
	if _, rows, _ := central.deployRows(); len(rows) != 3 {
		t.Fatalf("central screen should list all 3 deployments, got %d", len(rows))
	}
	proj := Model{snap: snap, screen: screenProject, tab: tabDeploys, scope: "/p/docs"}
	_, rows, _ := proj.deployRows()
	if len(rows) != 2 || rows[0].target != 1 {
		t.Fatalf("project tab should list only /p/docs, got %+v", rows)
	}
	empty := Model{snap: snap, screen: screenProject, tab: tabDeploys, scope: "/p/none"}
	lines, rows, _ := empty.deployRows()
	if len(rows) != 0 || !strings.Contains(strings.Join(lines, "\n"), "No Vercel") {
		t.Fatalf("empty project should explain itself: %v", lines)
	}
}
