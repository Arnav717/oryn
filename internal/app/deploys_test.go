package app

import (
	"github.com/Arnav717/oryn/internal/config"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/deploy"
	"github.com/Arnav717/oryn/internal/store"
)

func TestRecordDeployEvents(t *testing.T) {
	now := time.Now()
	snap := &Snapshot{Deploys: []DeployTarget{{
		Link: deploy.Link{Platform: "vercel", Project: "/p/web"},
		Deployments: []deploy.Deployment{
			{ID: "new-fail", Platform: "vercel", Target: "production", State: deploy.StateError, Created: now.Add(-time.Hour)},
			{ID: "old-fail", Platform: "vercel", Target: "production", State: deploy.StateError, Created: now.Add(-30 * 24 * time.Hour)},
			{ID: "ok", Platform: "vercel", Target: "production", State: deploy.StateReady, Created: now},
		},
	}}}
	var got []store.Event
	if err := recordDeployEvents(snap, func(e store.Event) error { got = append(got, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "deploy:vercel:new-fail" || got[0].Severity != 4 {
		t.Fatalf("got %+v", got)
	}
}

func TestWithConfigLinks(t *testing.T) {
	cfg := &config.Config{}
	cfg.SetDeployLink("/p/web", "vercel", "prj_ui", "team_1")
	cfg.SetDeployLink("/p/web", "netlify", "site_ui", "")
	cfg.SetDeployLink("/p/other", "vercel", "nope", "")
	found := []deploy.Link{{Platform: "vercel", Project: "/p/web", Marker: "vercel.json"}}
	got := withConfigLinks(found, cfg, "/p/web")
	if len(got) != 2 || got[0].Ref != "prj_ui" || got[0].Team != "team_1" || got[0].Marker != "oryn config" || got[1].Platform != "netlify" {
		t.Fatalf("got %+v", got)
	}
}
