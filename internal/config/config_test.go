package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReposRoundTrip(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(path, []byte("projects = [\"~/code/app\"]\ngithub = false\n[repos]\n\"~/code/app\" = \"o/r\"\n"), 0o644)

	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Repos[filepath.Join(home, "code/app")] != "o/r" || c.GitHubEnabled() {
		t.Fatalf("load: repos=%v github=%v", c.Repos, c.GitHubEnabled())
	}
	c.Projects = append(c.Projects, "/data/other")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c2.Projects) != 2 || c2.Repos[filepath.Join(home, "code/app")] != "o/r" || c2.GitHubEnabled() {
		t.Errorf("after save: projects=%v repos=%v github=%v", c2.Projects, c2.Repos, c2.GitHubEnabled())
	}
}
