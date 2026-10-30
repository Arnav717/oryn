package tui

import (
	"errors"
	"github.com/Arnav717/oryn/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
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
		cfg: cfg, screen: screenPicker, pickBack: screenDeploys, pickCursor: 2, // row 0 is "create new"
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
	dep := 0
	for _, r := range rows {
		if r.add == "" {
			dep++
			if r.target != 1 {
				t.Fatalf("project tab leaked another project's deployment: %+v", r)
			}
		}
	}
	if dep != 2 {
		t.Fatalf("project tab should list the 2 /p/docs deployments, got %+v", rows)
	}
	empty := Model{snap: snap, screen: screenProject, tab: tabDeploys, scope: "/p/none"}
	lines, rows, _ := empty.deployRows()
	if len(rows) != len(deploy.Platforms) || !strings.Contains(strings.Join(lines, "\n"), "No deploy config found") {
		t.Fatalf("empty project should explain itself: %v", lines)
	}
}

func TestProjectWithoutConfigOffersLinking(t *testing.T) {
	// No project anywhere has deploy config: the tab must still be actionable.
	cfg := &config.Config{Ephemeral: true}
	m := Model{cfg: cfg, snap: &app.Snapshot{}, screen: screenProject, tab: tabDeploys, scope: "/p/portfolio"}
	lines, rows, _ := m.deployRows()
	if len(rows) != len(deploy.Platforms) || rows[0].add != "vercel" {
		t.Fatalf("want one link row per platform, got %+v\n%s", rows, strings.Join(lines, "\n"))
	}
	// Enter on a platform with no account connected must go to Accounts, not do nothing.
	t.Setenv("VERCEL_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	deploy.SetCredsDir(t.TempDir())
	t.Cleanup(func() { deploy.SetCredsDir("") })
	next, _ := m.handleDeployKey(keyMsg("enter"))
	if got := next.(Model); got.screen != screenAccounts {
		t.Fatalf("enter should open Accounts, screen=%v flash=%q", got.screen, got.flash)
	}
	// With an account connected it opens the project picker and returns here after.
	t.Setenv("VERCEL_TOKEN", "tok")
	next, _ = m.handleDeployKey(keyMsg("l"))
	got := next.(Model)
	if got.screen != screenPicker || got.pickBack != screenProject || got.pickTarget.Link.Project != "/p/portfolio" {
		t.Fatalf("l should open the picker for this project: screen=%v back=%v target=%+v", got.screen, got.pickBack, got.pickTarget.Link)
	}
	// The central screen with nothing detected explains itself.
	central := Model{snap: &app.Snapshot{}, screen: screenDeploys}
	if lines, rows, _ := central.deployRows(); len(rows) != 0 || len(lines) == 0 {
		t.Errorf("central empty state: %v", lines)
	}
}

func TestCreateProjectFlow(t *testing.T) {
	cfg := &config.Config{Ephemeral: true}
	m := Model{
		cfg: cfg, screen: screenPicker, pickBack: screenProject,
		pickTarget: app.DeployTarget{Link: deploy.Link{Platform: "vercel", Project: "/code/My Portfolio"}},
	}
	// The first row creates; enter on it asks for a name, prefilled from the folder.
	next, _ := m.handlePickerKey(keyMsg("enter"))
	m = next.(Model)
	if !m.creating || m.createInput.Value() != "my-portfolio" {
		t.Fatalf("creating=%v name=%q", m.creating, m.createInput.Value())
	}
	// A bad name is refused, with no confirmation step.
	m.createInput.SetValue("Bad Name")
	next, _ = m.handlePickerKey(keyMsg("enter"))
	m = next.(Model)
	if m.createConfirm || !m.creating {
		t.Fatal("invalid name must not reach confirmation")
	}
	m.createInput.SetValue("my-portfolio")
	next, _ = m.handlePickerKey(keyMsg("enter"))
	m = next.(Model)
	if !m.createConfirm || m.createName != "my-portfolio" {
		t.Fatalf("want confirmation, got %+v", m.createConfirm)
	}
	// Anything but y cancels without creating.
	next, cmd := m.handlePickerKey(keyMsg("x"))
	if cmd != nil || next.(Model).createConfirm {
		t.Fatal("non-y must cancel")
	}
	// y starts creation (a command to run); nothing is linked until it succeeds.
	next, cmd = m.handlePickerKey(keyMsg("y"))
	if cmd == nil || len(cfg.DeployLinks) != 0 {
		t.Fatalf("y should start creation only: cmd=%v links=%v", cmd != nil, cfg.DeployLinks)
	}
	// When the platform confirms, the project is linked and we return where we were.
	out, _ := next.(Model).Update(createdMsg{platform: "vercel", ref: deploy.ProjectRef{Name: "my-portfolio", Ref: "prj_9", Team: "usr_1"}})
	got := out.(Model)
	if got.screen != screenProject || len(cfg.DeployLinks) != 1 || cfg.DeployLinks[0].Ref != "prj_9" || !strings.Contains(got.flash, "press p to deploy") {
		t.Fatalf("screen=%v links=%+v flash=%q", got.screen, cfg.DeployLinks, got.flash)
	}
	// A failed create links nothing.
	out, _ = m.Update(createdMsg{platform: "vercel", err: errors.New("name taken")})
	if len(cfg.DeployLinks) != 1 || !strings.Contains(out.(Model).flash, "name taken") {
		t.Fatal("failed create must not link")
	}
}

func TestDeployNeedsConfirmation(t *testing.T) {
	t.Setenv("VERCEL_TOKEN", "tok")
	t.Setenv("PATH", t.TempDir()) // no CLIs installed
	target := app.DeployTarget{Link: deploy.Link{Platform: "vercel", Project: t.TempDir(), Ref: "prj", Team: "usr"}}
	m := Model{screen: screenDeploys}
	next, cmd := m.askDeploy(target)
	if got := next.(Model); got.deployAsk != nil || cmd != nil || !strings.Contains(got.flash, "npm i -g vercel") {
		t.Fatalf("missing CLI should explain how to install: %q", got.flash)
	}
	// With a CLI present, a prompt is shown and nothing runs until y.
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "vercel"), []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", bin)
	next, cmd = m.askDeploy(target)
	m = next.(Model)
	if m.deployAsk == nil || cmd != nil {
		t.Fatal("should wait for confirmation without running anything")
	}
	if strings.Contains(m.deployAsk.Display(), "tok") && strings.Contains(m.deployAsk.Display(), "VERCEL_TOKEN") {
		t.Error("token leaked into the prompt")
	}
	next, cmd = m.handleDeployConfirm(keyMsg("n"))
	if cmd != nil || next.(Model).deployAsk != nil {
		t.Fatal("n must cancel")
	}
	_, cmd = m.handleDeployConfirm(keyMsg("y"))
	if cmd == nil {
		t.Fatal("y should start the deploy")
	}
}
