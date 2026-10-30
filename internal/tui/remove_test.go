package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/deploy"
)

func deleteModel(cfg *config.Config) Model {
	link := deploy.Link{Platform: "vercel", Project: "/p/web", Ref: "prj_1", Team: "team_1", Name: "web-site"}
	return Model{
		cfg: cfg, screen: screenDeploys, now: time.Now(), loading: true, // loading: refreshes only queue
		snap: &app.Snapshot{Deploys: []app.DeployTarget{{
			Link: link, Queryable: true,
			Deployments: []deploy.Deployment{{ID: "dpl_abcdefghijklmnop", Platform: "vercel", Target: "production", State: deploy.StateReady, Branch: "main"}},
		}}},
	}
}

func TestDeleteDeploymentNeedsExplicitYes(t *testing.T) {
	m := deleteModel(&config.Config{Ephemeral: true})
	next, cmd := m.handleDeployKey(keyMsg("d"))
	m = next.(Model)
	if m.delAsk == nil || m.delAsk.project || cmd != nil {
		t.Fatalf("d should ask, not delete: ask=%+v cmd=%v", m.delAsk, cmd != nil)
	}
	if p := m.deletePrompt(); !strings.Contains(p, "production") || !strings.Contains(p, "main") {
		t.Errorf("prompt should say it's the production deployment: %q", p)
	}
	next, cmd = m.handleKey(keyMsg("n"))
	if got := next.(Model); got.delAsk != nil || cmd != nil || !strings.Contains(got.flash, "nothing deleted") {
		t.Fatalf("any key but y must cancel: %+v", got.flash)
	}
	next, cmd = m.handleKey(keyMsg("y"))
	if next.(Model).delAsk != nil || cmd == nil {
		t.Fatal("y should start the delete")
	}
}

func TestDeleteProjectRequiresTypingTheName(t *testing.T) {
	cfg := &config.Config{Ephemeral: true}
	cfg.SetDeployLink("/p/web", "vercel", "prj_1", "team_1", "web-site")
	m := deleteModel(cfg)
	next, _ := m.handleDeployKey(keyMsg("X"))
	m = next.(Model)
	if m.delAsk == nil || !m.delAsk.project || m.delAsk.word != "web-site" {
		t.Fatalf("X should ask for the project name: %+v", m.delAsk)
	}
	// Enter with nothing typed, or the wrong name, must not delete.
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || next.(Model).delAsk == nil {
		t.Fatal("empty confirmation must not delete")
	}
	for _, r := range "wrong" {
		next, _ = next.(Model).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	next, cmd = next.(Model).handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || next.(Model).delAsk == nil || !strings.Contains(next.(Model).flash, "doesn't match") {
		t.Fatalf("wrong name must not delete: %q", next.(Model).flash)
	}
	// esc cancels everything.
	next, cmd = next.(Model).handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || next.(Model).delAsk != nil || len(cfg.DeployLinks) != 1 {
		t.Fatal("esc must cancel and keep the link")
	}
	// The exact name deletes.
	next, _ = m.handleDeployKey(keyMsg("X"))
	m = next.(Model)
	m.delAsk.input.SetValue("web-site")
	next, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || next.(Model).delAsk != nil {
		t.Fatal("the exact name should start the delete")
	}
}

func TestDeleteResults(t *testing.T) {
	cfg := &config.Config{Ephemeral: true}
	cfg.SetDeployLink("/p/web", "vercel", "prj_1", "team_1", "web-site")
	m := deleteModel(cfg)
	ask := deleteAsk{project: true, target: m.snap.Deploys[0], word: "web-site"}

	// A failure keeps the link and says why.
	out, _ := m.Update(deletedMsg{ask, errors.New("cannot delete active deployment")})
	if len(cfg.DeployLinks) != 1 || !strings.Contains(out.(Model).flash, "cannot delete active") {
		t.Fatalf("failed delete: links=%d flash=%q", len(cfg.DeployLinks), out.(Model).flash)
	}
	// Success removes the link.
	out, _ = m.Update(deletedMsg{ask, nil})
	if len(cfg.DeployLinks) != 0 || !strings.Contains(out.(Model).flash, "deleted vercel project web-site") {
		t.Fatalf("deleted project: links=%d flash=%q", len(cfg.DeployLinks), out.(Model).flash)
	}
	if out.(Model).keepFlash == "" {
		t.Error("result must survive the refresh")
	}
}

func TestUnlinkOnlyForgetsLocally(t *testing.T) {
	cfg := &config.Config{Ephemeral: true}
	cfg.SetDeployLink("/p/web", "vercel", "prj_1", "team_1", "web-site")
	m := deleteModel(cfg)
	next, cmd := m.handleDeployKey(keyMsg("u"))
	if len(cfg.DeployLinks) != 0 || !strings.Contains(next.(Model).flash, "still exists there") {
		t.Fatalf("unlink: links=%d flash=%q", len(cfg.DeployLinks), next.(Model).flash)
	}
	_ = cmd
	// Nothing to unlink when the platform came from a config file.
	next, _ = m.handleDeployKey(keyMsg("u"))
	if !strings.Contains(next.(Model).flash, "no oryn link") {
		t.Errorf("flash = %q", next.(Model).flash)
	}
}

func TestCloudflareLiveDeploymentExplainsItself(t *testing.T) {
	live := deploy.Deployment{ID: "live", Target: "production", State: deploy.StateReady}
	failed := deploy.Deployment{ID: "new", Target: "production", State: deploy.StateError}
	old := deploy.Deployment{ID: "old", Target: "production", State: deploy.StateReady}
	preview := deploy.Deployment{ID: "pre", Target: "preview", State: deploy.StateReady}
	tgt := app.DeployTarget{Link: deploy.Link{Platform: "cloudflare", Project: "/p", Ref: "site"}, Queryable: true,
		Deployments: []deploy.Deployment{failed, preview, live, old}}
	m := Model{snap: &app.Snapshot{Deploys: []app.DeployTarget{tgt}}, screen: screenDeploys, w: 200, h: 40}

	for i, d := range tgt.Deployments {
		m.dpCursor = i
		next, _ := m.askDeleteDeployment()
		got := next.(Model)
		if d.ID == "live" {
			if got.delAsk != nil || !strings.Contains(got.flash, "press X") {
				t.Errorf("live: ask %+v flash %q", got.delAsk, got.flash)
			}
		} else if got.delAsk == nil {
			t.Errorf("%s should be deletable: flash %q", d.ID, got.flash)
		}
	}

	// Vercel has no such rule.
	tgt.Link.Platform = "vercel"
	m.snap.Deploys[0] = tgt
	m.dpCursor = 2
	if next, _ := m.askDeleteDeployment(); next.(Model).delAsk == nil {
		t.Error("vercel live deployment should still ask")
	}
}
