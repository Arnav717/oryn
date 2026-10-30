// Package deploy finds where a project is deployed and talks to hosting
// platforms. Detection is local-only; credentials are read per request from
// the environment or the platform CLI's own login and never stored.
package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Supported platforms: Vercel, Netlify and Cloudflare Pages. More are
// deliberately out of scope until these three are solid.

// Link ties a project folder to one deployment target on a platform.
type Link struct {
	Platform string // vercel, netlify, cloudflare
	Project  string // watched project root
	Ref      string // platform's project/site/app identifier; "" when only a config file was found
	Team     string // org or team id when the platform needs one
	Marker   string // file that revealed the platform
}

// Linked reports whether the project is bound to a concrete target oryn can query.
func (l Link) Linked() bool { return l.Ref != "" }

// Detect returns every platform the project at root is configured for.
func Detect(root string) []Link {
	var out []Link
	add := func(l Link) {
		l.Project = root
		out = append(out, l)
	}
	exists := func(rel string) (string, bool) {
		p := filepath.Join(root, rel)
		_, err := os.Stat(p)
		return p, err == nil
	}

	if p, ok := exists(".vercel/project.json"); ok {
		var v struct {
			ProjectID string `json:"projectId"`
			OrgID     string `json:"orgId"`
		}
		readJSON(p, &v)
		add(Link{Platform: "vercel", Ref: v.ProjectID, Team: v.OrgID, Marker: ".vercel/project.json"})
	} else if _, ok := exists("vercel.json"); ok {
		add(Link{Platform: "vercel", Marker: "vercel.json"})
	}

	if p, ok := exists(".netlify/state.json"); ok {
		var v struct {
			SiteID string `json:"siteId"`
		}
		readJSON(p, &v)
		add(Link{Platform: "netlify", Ref: v.SiteID, Marker: ".netlify/state.json"})
	} else if _, ok := exists("netlify.toml"); ok {
		add(Link{Platform: "netlify", Marker: "netlify.toml"})
	}

	for _, f := range []string{"wrangler.toml", "wrangler.json", "wrangler.jsonc"} {
		if p, ok := exists(f); ok {
			l := Link{Platform: "cloudflare", Ref: wranglerName(p), Marker: f}
			if !wranglerIsPages(p) {
				l.Ref = "" // plain Workers: detected, but not queryable yet
			}
			add(l)
			break
		}
	}

	return dedupe(out)
}

func dedupe(in []Link) []Link {
	seen := map[string]bool{}
	var out []Link
	for _, l := range in {
		if !seen[l.Platform] {
			seen[l.Platform] = true
			out = append(out, l)
		}
	}
	return out
}

func wranglerIsPages(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), "pages_build_output_dir")
}

func readJSON(path string, v any) {
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, v)
	}
}

var wranglerNameRE = regexp.MustCompile(`"?name"?\s*[=:]\s*"([^"]+)"`)

// wranglerName pulls the worker/pages name from wrangler.toml/json(c). A
// regexp is enough here: the name is a top-level string and JSONC comments
// would break a strict parser.
func wranglerName(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if m := wranglerNameRE.FindSubmatch(b); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}
