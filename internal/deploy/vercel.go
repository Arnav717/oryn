package deploy

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Arnav717/oryn/internal/source"
)

type Vercel struct{ base string }

func NewVercel() *Vercel { return &Vercel{base: "https://api.vercel.com"} }

func (*Vercel) Name() string { return "vercel" }

// vercelToken reads VERCEL_TOKEN or the token the Vercel CLI saved on login.
func vercelToken() string {
	if t := os.Getenv("VERCEL_TOKEN"); t != "" {
		return t
	}
	if t := stored("vercel_token"); t != "" {
		return t
	}
	dirs := []string{}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		dirs = append(dirs, d)
	}
	if h, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(h, ".local", "share"), filepath.Join(h, "Library", "Application Support"))
	}
	for _, d := range dirs {
		var v struct {
			Token string `json:"token"`
		}
		if b, err := os.ReadFile(filepath.Join(d, "com.vercel.cli", "auth.json")); err == nil && json.Unmarshal(b, &v) == nil && v.Token != "" {
			return v.Token
		}
	}
	return ""
}

func (v *Vercel) List(ctx context.Context, l Link, limit int) ([]Deployment, error) {
	tok := vercelToken()
	if tok == "" {
		return nil, ErrNoCredentials
	}
	q := url.Values{"projectId": {l.Ref}, "limit": {strconv.Itoa(limit)}}
	if strings.HasPrefix(l.Team, "team_") {
		q.Set("teamId", l.Team)
	}
	var r struct {
		Deployments []struct {
			UID     string `json:"uid"`
			URL     string `json:"url"`
			State   string `json:"state"`
			Target  string `json:"target"`
			Created int64  `json:"created"`
			Meta    struct {
				SHA     string `json:"githubCommitSha"`
				Message string `json:"githubCommitMessage"`
				Ref     string `json:"githubCommitRef"`
			} `json:"meta"`
		} `json:"deployments"`
	}
	err := source.DoJSONH(ctx, "GET", v.base+"/v6/deployments?"+q.Encode(),
		map[string]string{"Authorization": "Bearer " + tok}, nil, &r)
	if err != nil {
		return nil, err
	}
	out := make([]Deployment, 0, len(r.Deployments))
	for _, d := range r.Deployments {
		target := "preview"
		if d.Target == "production" {
			target = "production"
		}
		out = append(out, Deployment{
			ID: d.UID, Platform: "vercel", Target: target, State: vercelState(d.State),
			Branch: d.Meta.Ref, Commit: shortSHA(d.Meta.SHA), Message: firstLine(d.Meta.Message),
			URL: "https://" + d.URL, Created: time.UnixMilli(d.Created),
		})
	}
	return out, nil
}

func (v *Vercel) LiveURL(ctx context.Context, l Link) (string, error) {
	tok := vercelToken()
	if tok == "" {
		return "", ErrNoCredentials
	}
	var r struct {
		Domains []struct {
			Name      string `json:"name"`
			Verified  bool   `json:"verified"`
			Redirect  string `json:"redirect"`
			GitBranch string `json:"gitBranch"`
		} `json:"domains"`
	}
	u := v.base + "/v9/projects/" + url.PathEscape(l.Ref) + "/domains" + vercelTeamQuery(l)
	if err := source.DoJSONH(ctx, "GET", u, map[string]string{"Authorization": "Bearer " + tok}, nil, &r); err != nil {
		return "", err
	}
	var names []string
	for _, d := range r.Domains {
		if d.Verified && d.Redirect == "" && d.GitBranch == "" {
			names = append(names, d.Name)
		}
	}
	return preferCustom(names, ".vercel.app"), nil
}

// preferCustom picks the first domain that isn't on the platform's own suffix,
// falling back to the first of any kind, as an https URL.
func preferCustom(names []string, platformSuffix string) string {
	if len(names) == 0 {
		return ""
	}
	for _, n := range names {
		if !strings.HasSuffix(n, platformSuffix) {
			return "https://" + n
		}
	}
	return "https://" + names[0]
}

func vercelState(s string) string {
	switch s {
	case "READY":
		return StateReady
	case "ERROR":
		return StateError
	case "CANCELED":
		return StateCanceled
	case "QUEUED":
		return StateQueued
	}
	return StateBuilding // BUILDING, INITIALIZING and anything new
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
