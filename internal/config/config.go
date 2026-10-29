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
}

type Config struct {
	Projects []string  `toml:"projects"`
	Services []Service `toml:"service"`
	Refresh  string    `toml:"refresh"`

	RefreshEvery   time.Duration `toml:"-"`
	Path           string        `toml:"-"`
	customServices bool

	// Ephemeral marks a config whose projects came from -p and must not be saved.
	Ephemeral bool `toml:"-"`
}

var DefaultServices = []Service{
	{"GitHub", "https://www.githubstatus.com"},
	{"npm", "https://status.npmjs.org"},
	{"Cloudflare", "https://www.cloudflarestatus.com"},
	{"Claude", "https://status.claude.com"},
	{"OpenAI", "https://status.openai.com"},
	{"Vercel", "https://www.vercel-status.com"},
	{"Netlify", "https://www.netlifystatus.com"},
	{"DigitalOcean", "https://status.digitalocean.com"},
	{"Supabase", "https://status.supabase.com"},
	{"Docker", "https://www.dockerstatus.com"},
	{"Discord", "https://discordstatus.com"},
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
	return c, nil
}

// Save writes the config back to Path. Built-in services are not persisted.
func (c *Config) Save() error {
	out := struct {
		Projects []string  `toml:"projects"`
		Refresh  string    `toml:"refresh,omitempty"`
		Services []Service `toml:"service,omitempty"`
	}{Refresh: c.Refresh}
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
