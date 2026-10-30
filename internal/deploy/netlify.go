package deploy

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Arnav717/oryn/internal/source"
)

type Netlify struct{ base string }

func NewNetlify() *Netlify { return &Netlify{base: "https://api.netlify.com"} }

func (*Netlify) Name() string { return "netlify" }

// netlifyToken reads NETLIFY_AUTH_TOKEN or the login the Netlify CLI saved.
func netlifyToken() string {
	if t := os.Getenv("NETLIFY_AUTH_TOKEN"); t != "" {
		return t
	}
	if t := stored("netlify_token"); t != "" {
		return t
	}
	dirs := []string{}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		dirs = append(dirs, d)
	}
	if h, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(h, ".config"), filepath.Join(h, "Library", "Preferences"))
	}
	for _, d := range dirs {
		var v struct {
			Users map[string]struct {
				Auth struct {
					Token string `json:"token"`
				} `json:"auth"`
			} `json:"users"`
		}
		if b, err := os.ReadFile(filepath.Join(d, "netlify", "config.json")); err == nil && json.Unmarshal(b, &v) == nil {
			for _, u := range v.Users {
				if u.Auth.Token != "" {
					return u.Auth.Token
				}
			}
		}
	}
	return ""
}

func (n *Netlify) List(ctx context.Context, l Link, limit int) ([]Deployment, error) {
	tok := netlifyToken()
	if tok == "" {
		return nil, ErrNoCredentials
	}
	var r []struct {
		ID        string    `json:"id"`
		State     string    `json:"state"`
		Context   string    `json:"context"`
		Branch    string    `json:"branch"`
		CommitRef string    `json:"commit_ref"`
		Title     string    `json:"title"`
		SSLURL    string    `json:"ssl_url"`
		DeployURL string    `json:"deploy_ssl_url"`
		Created   time.Time `json:"created_at"`
	}
	u := n.base + "/api/v1/sites/" + url.PathEscape(l.Ref) + "/deploys?per_page=" + strconv.Itoa(limit)
	if err := source.DoJSONH(ctx, "GET", u, map[string]string{"Authorization": "Bearer " + tok}, nil, &r); err != nil {
		return nil, err
	}
	out := make([]Deployment, 0, len(r))
	for _, d := range r {
		target := "preview"
		if d.Context == "production" {
			target = "production"
		}
		link := d.DeployURL
		if link == "" {
			link = d.SSLURL
		}
		out = append(out, Deployment{
			ID: d.ID, Platform: "netlify", Target: target, State: netlifyState(d.State),
			Branch: d.Branch, Commit: shortSHA(d.CommitRef), Message: firstLine(d.Title),
			URL: link, Created: d.Created.Local(),
		})
	}
	return out, nil
}

func netlifyState(s string) string {
	switch s {
	case "ready":
		return StateReady
	case "error":
		return StateError
	case "enqueued", "new":
		return StateQueued
	case "rejected", "skipped", "canceled":
		return StateCanceled
	}
	return StateBuilding // building, uploading, processing, ...
}
