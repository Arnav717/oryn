package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/gitrepo"
)

func finderModel(t *testing.T) Model {
	t.Helper()
	m := shellModel()
	m.cfg.Path = filepath.Join(t.TempDir(), "config.toml")
	m.screen = screenProjects
	f := &finder{picked: map[string]bool{}, listed: true, root: "/code", repos: []gitrepo.Found{
		{Dir: "/code/a", Ref: gitrepo.Ref{Owner: "o", Name: "a"}, GitHub: true}, // already watched
		{Dir: "/code/c", Ref: gitrepo.Ref{Owner: "o", Name: "c"}, GitHub: true},
		{Dir: "/code/d"},
	}}
	m.finder = f
	return m
}

func TestFinderOpensFromProjects(t *testing.T) {
	m := shellModel()
	m.cwd = "/code/somewhere"
	got := press(t, m, "2", "f")
	if got.finder == nil || !strings.Contains(got.View(), "Folder to look in") {
		t.Fatalf("view:\n%s", got.View())
	}
	if got = press(t, got, "esc"); got.finder != nil {
		t.Error("esc should close it")
	}
}

func TestFinderSearchesRealFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := shellModel()
	m.cwd = root
	next, _ := m.openFinder()
	m = next.(Model)
	next, cmd := m.handleKey(keyMsg("enter"))
	m = next.(Model)
	if !m.finder.loading || cmd == nil {
		t.Fatalf("not searching: %+v", m.finder)
	}
	next, _ = m.Update(cmd())
	got := next.(Model)
	if !got.finder.listed || len(got.finder.repos) != 1 {
		t.Fatalf("found: %+v", got.finder)
	}
}

func TestFinderBadFolderKeepsTheInput(t *testing.T) {
	m := shellModel()
	next, _ := m.openFinder()
	m = next.(Model)
	m.finder.root, m.finder.loading = "/nope", true
	next, _ = m.Update(foundMsg{root: "/nope", err: os.ErrNotExist})
	got := next.(Model)
	if got.finder == nil || got.finder.listed || got.finder.loading || got.flash == "" {
		t.Errorf("after error: %+v flash %q", got.finder, got.flash)
	}
}

func TestFinderPicksAndWatches(t *testing.T) {
	m := finderModel(t)
	out := m.View()
	for _, want := range []string{"o/c", "no GitHub remote", "2 GitHub, 1 other", "✓"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	// Already watched can't be picked.
	m = press(t, m, " ")
	if len(m.finder.picked) != 0 {
		t.Errorf("picked a watched repo: %v", m.finder.picked)
	}
	m = press(t, m, "j", " ")
	if !m.finder.picked["/code/c"] {
		t.Fatalf("picked: %v", m.finder.picked)
	}
	got := press(t, m, "enter")
	if got.finder != nil || got.screen != screenProjects || !strings.Contains(got.flash, "1 more") {
		t.Fatalf("after enter: %+v %q", got.finder, got.flash)
	}
	if len(got.cfg.Projects) != 3 || got.cfg.Projects[2] != "/code/c" {
		t.Errorf("projects: %v", got.cfg.Projects)
	}
	saved, err := config.Load(got.cfg.Path)
	if err != nil || len(saved.Projects) != 3 {
		t.Errorf("not saved: %v %v", saved, err)
	}
}

func TestFinderPickAll(t *testing.T) {
	m := press(t, finderModel(t), "a")
	if len(m.finder.picked) != 2 || m.finder.picked["/code/a"] {
		t.Errorf("all: %v", m.finder.picked)
	}
	m = press(t, m, "a")
	if len(m.finder.picked) != 0 {
		t.Errorf("a again should clear: %v", m.finder.picked)
	}
	if got := press(t, m, "enter"); got.finder == nil || !strings.Contains(got.flash, "nothing picked") {
		t.Errorf("enter with none: %q", got.flash)
	}
}

func TestFinderKeysDontSwitchTabs(t *testing.T) {
	m := finderModel(t)
	m.finder.listed = false
	got := press(t, m, "3")
	if got.screen != screenProjects {
		t.Errorf("a number key left the finder: %d", got.screen)
	}
}

func TestFinderRespectsOneOffProjects(t *testing.T) {
	m := shellModel()
	m.cfg.Ephemeral = true
	if got := press(t, m, "2", "f"); got.finder != nil || !strings.Contains(got.flash, "-p") {
		t.Errorf("ephemeral: %+v %q", got.finder, got.flash)
	}
}

func TestFinderScrolls(t *testing.T) {
	m := finderModel(t)
	m.h = 14
	m.finder.repos = nil
	for range 40 {
		m.finder.repos = append(m.finder.repos, gitrepo.Found{Dir: "/x"})
	}
	for range 39 {
		m = press(t, m, "j")
	}
	if !strings.Contains(strings.Join(m.finderLines(), "\n"), "▸") {
		t.Errorf("cursor scrolled away:\n%s", strings.Join(m.finderLines(), "\n"))
	}
	if n := len(strings.Split(m.View(), "\n")); n > m.h {
		t.Errorf("%d lines", n)
	}
}
