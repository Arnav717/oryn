package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/deploy"
	"github.com/Arnav717/oryn/internal/store"
)

// DeployTarget is one project/platform pair: what was detected and, when it
// could be queried, its recent deployments.
type DeployTarget struct {
	Link        deploy.Link
	Deployments []deploy.Deployment // newest first
	Err         error               // deploy.ErrNoCredentials when no token was found
	Queryable   bool                // a provider exists and the project is linked
}

// Latest returns the newest deployment for target ("production"/"preview"), if any.
func (t DeployTarget) Latest(target string) (deploy.Deployment, bool) {
	for _, d := range t.Deployments {
		if d.Target == target {
			return d, true
		}
	}
	return deploy.Deployment{}, false
}

func fetchDeploys(ctx context.Context, cfg *config.Config, snap *Snapshot) {
	var targets []DeployTarget
	for _, p := range cfg.Projects {
		for _, l := range deploy.Detect(p) {
			prov := deploy.ProviderFor(l.Platform)
			targets = append(targets, DeployTarget{Link: l, Queryable: prov != nil && l.Linked()})
		}
	}
	var wg sync.WaitGroup
	for i := range targets {
		if !targets[i].Queryable {
			continue
		}
		wg.Add(1)
		go func(t *DeployTarget) {
			defer wg.Done()
			t.Deployments, t.Err = deploy.ProviderFor(t.Link.Platform).List(ctx, t.Link, 10)
		}(&targets[i])
	}
	wg.Wait()
	snap.Deploys = targets
}

// deployFailureWindow bounds how old a failed deploy can be and still alert,
// so connecting a platform doesn't dump months of history on the tape.
const deployFailureWindow = 48 * time.Hour

func recordDeployEvents(snap *Snapshot, record func(store.Event) error) error {
	for _, t := range snap.Deploys {
		for _, d := range t.Deployments {
			if d.State != deploy.StateError || time.Since(d.Created) > deployFailureWindow {
				continue
			}
			err := record(store.Event{
				Key:      fmt.Sprintf("deploy:%s:%s", d.Platform, d.ID),
				Kind:     "deploy",
				Severity: map[bool]int{true: 4, false: 2}[d.Target == "production"],
				Entity:   t.Link.Project,
				Title:    fmt.Sprintf("%s deploy failed (%s %s) %s", d.Platform, d.Target, d.Branch, d.Message),
				URL:      d.URL,
				At:       d.Created,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
