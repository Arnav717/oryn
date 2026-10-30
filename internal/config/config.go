// Package config loads ~/.config/oryn/config.toml.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Service struct {
	Name string `toml:"name"`
	URL  string `toml:"url"` // statuspage base URL, e.g. https://www.githubstatus.com
	Via  string `toml:"-"`   // package that pulled this service in (stack-detected only)
}

type Config struct {
	Projects []string  `toml:"projects"`
	Services []Service `toml:"service"`
	Refresh  string    `toml:"refresh"`
	GitHub   *bool     `toml:"github"` // nil = on whenever a token is available
	// StackStatus adds status pages for vendors found in your lockfiles (nil = on).
	StackStatus *bool `toml:"stack_status"`

	// Repos overrides the GitHub repo detected from a project's git remote:
	// [repos] "~/code/app" = "owner/name".
	Repos map[string]string `toml:"repos"`

	RefreshEvery   time.Duration `toml:"-"`
	Path           string        `toml:"-"`
	customServices bool

	// Ephemeral marks a config whose projects came from -p and must not be saved.
	Ephemeral bool `toml:"-"`
}

var DefaultServices = []Service{
	{Name: "GitHub", URL: "https://www.githubstatus.com"},
	{Name: "npm", URL: "https://status.npmjs.org"},
	{Name: "Cloudflare", URL: "https://www.cloudflarestatus.com"},
	{Name: "Claude", URL: "https://status.claude.com"},
	{Name: "OpenAI", URL: "https://status.openai.com"},
	{Name: "Vercel", URL: "https://www.vercel-status.com"},
	{Name: "Netlify", URL: "https://www.netlifystatus.com"},
	{Name: "DigitalOcean", URL: "https://status.digitalocean.com"},
	{Name: "Supabase", URL: "https://status.supabase.com"},
	{Name: "Docker", URL: "https://www.dockerstatus.com"},
	{Name: "Discord", URL: "https://discordstatus.com"},
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "oryn", "config.toml")
}

// Load reads the config at path. A missing file is not an error.
func Load(path string) (*Config, error) {
	c := &Config{Path: path}
	if path != "" {
		if _, err := toml.DecodeFile(path, c); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	c.customServices = len(c.Services) > 0
	if !c.customServices {
		c.Services = DefaultServices
	}
	c.RefreshEvery = 10 * time.Minute
	if c.Refresh != "" {
		d, err := time.ParseDuration(c.Refresh)
		if err != nil {
			return nil, err
		}
		c.RefreshEvery = d
	}
	for i, p := range c.Projects {
		c.Projects[i] = ExpandHome(p)
	}
	if len(c.Repos) > 0 {
		expanded := make(map[string]string, len(c.Repos))
		for path, repo := range c.Repos {
			expanded[ExpandHome(path)] = repo
		}
		c.Repos = expanded
	}
	return c, nil
}

// GitHubEnabled reports whether the GitHub integration is switched on.
func (c *Config) GitHubEnabled() bool { return c.GitHub == nil || *c.GitHub }

// StackStatusEnabled reports whether lockfile-derived status pages are on.
func (c *Config) StackStatusEnabled() bool { return c.StackStatus == nil || *c.StackStatus }

// Save writes the config back to Path. Built-in services are not persisted.
func (c *Config) Save() error {
	out := struct {
		Projects    []string          `toml:"projects"`
		Refresh     string            `toml:"refresh,omitempty"`
		GitHub      *bool             `toml:"github,omitempty"`
		StackStatus *bool             `toml:"stack_status,omitempty"`
		Services    []Service         `toml:"service,omitempty"`
		Repos       map[string]string `toml:"repos,omitempty"`
	}{Refresh: c.Refresh, GitHub: c.GitHub, StackStatus: c.StackStatus}
	if len(c.Repos) > 0 {
		out.Repos = map[string]string{}
		for path, repo := range c.Repos {
			out.Repos[ShortHome(path)] = repo
		}
	}
	for _, p := range c.Projects {
		out.Projects = append(out.Projects, ShortHome(p))
	}
	if c.customServices {
		out.Services = c.Services
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(c.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(out)
}

// ShortHome replaces the home directory prefix with ~.
func ShortHome(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + p[len(home):]
	}
	return p
}

func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}
