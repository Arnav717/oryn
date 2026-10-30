package deploy

import (
	"context"
	"net/url"
	"os"
	"time"

	"github.com/Arnav717/oryn/internal/source"
)

// Cloudflare reads Cloudflare Pages deployments. It needs an API token and
// account id (CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID); wrangler's OAuth
// login is not reused because its token format is not a stable interface.
type Cloudflare struct{ base string }

func NewCloudflare() *Cloudflare { return &Cloudflare{base: "https://api.cloudflare.com"} }

func (*Cloudflare) Name() string { return "cloudflare" }

func (c *Cloudflare) List(ctx context.Context, l Link, limit int) ([]Deployment, error) {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return nil, ErrNoCredentials
	}
	var r struct {
		Result []struct {
			ID          string    `json:"id"`
			Environment string    `json:"environment"`
			URL         string    `json:"url"`
			Created     time.Time `json:"created_on"`
			Trigger     struct {
				Metadata struct {
					Branch  string `json:"branch"`
					Hash    string `json:"commit_hash"`
					Message string `json:"commit_message"`
				} `json:"metadata"`
			} `json:"deployment_trigger"`
			Stage struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"latest_stage"`
		} `json:"result"`
	}
	u := c.base + "/client/v4/accounts/" + url.PathEscape(acc) + "/pages/projects/" + url.PathEscape(l.Ref) + "/deployments"
	if err := source.DoJSONH(ctx, "GET", u, map[string]string{"Authorization": "Bearer " + tok}, nil, &r); err != nil {
		return nil, err
	}
	out := make([]Deployment, 0, len(r.Result))
	for _, d := range r.Result {
		if len(out) == limit {
			break
		}
		target := "preview"
		if d.Environment == "production" {
			target = "production"
		}
		m := d.Trigger.Metadata
		out = append(out, Deployment{
			ID: d.ID, Platform: "cloudflare", Target: target, State: cloudflareState(d.Stage.Name, d.Stage.Status),
			Branch: m.Branch, Commit: shortSHA(m.Hash), Message: firstLine(m.Message),
			URL: d.URL, Created: d.Created.Local(),
		})
	}
	return out, nil
}

func (c *Cloudflare) LiveURL(ctx context.Context, l Link) (string, error) {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return "", ErrNoCredentials
	}
	auth := map[string]string{"Authorization": "Bearer " + tok}
	base := c.base + "/client/v4/accounts/" + url.PathEscape(acc) + "/pages/projects/" + url.PathEscape(l.Ref)
	var p struct {
		Result struct {
			Subdomain string `json:"subdomain"`
		} `json:"result"`
	}
	if err := source.DoJSONH(ctx, "GET", base, auth, nil, &p); err != nil {
		return "", err
	}
	// The project's own domain list includes ones still pending, so ask for
	// status. If that fails the pages.dev address is still right.
	var d struct {
		Result []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"result"`
	}
	var names []string
	if err := source.DoJSONH(ctx, "GET", base+"/domains", auth, nil, &d); err == nil {
		for _, x := range d.Result {
			if x.Status == "active" {
				names = append(names, x.Name)
			}
		}
	}
	if p.Result.Subdomain != "" {
		names = append(names, p.Result.Subdomain)
	}
	return preferCustom(names, ".pages.dev"), nil
}

// cloudflareState: a deployment is only ready once its final "deploy" stage succeeded.
func cloudflareState(stage, status string) string {
	switch status {
	case "failure":
		return StateError
	case "canceled":
		return StateCanceled
	case "idle":
		return StateQueued
	case "success":
		if stage == "deploy" {
			return StateReady
		}
	}
	return StateBuilding
}

// cloudflareCreds returns the API token and account id, env first.
func cloudflareCreds() (token, account string) {
	token, account = os.Getenv("CLOUDFLARE_API_TOKEN"), os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if token == "" {
		token = stored("cloudflare_token")
	}
	if account == "" {
		account = stored("cloudflare_account")
	}
	return token, account
}
