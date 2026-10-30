package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/gitrepo"
)

// finder lists the git repositories in a folder to pick which to watch.
type finder struct {
	input   textinput.Model
	root    string
	loading bool
	listed  bool
	repos   []gitrepo.Found
	picked  map[string]bool
	cursor  int
	offset  int
}

type foundMsg struct {
	root  string
	repos []gitrepo.Found
	err   error
}

func (f finder) hints() string {
	if f.listed {
		return "space pick · a all · enter watch picked · esc back"
	}
	return "enter search · esc back"
}

// openFinder starts from the folder holding the current project, which is
// usually where the others live.
func (m Model) openFinder() (tea.Model, tea.Cmd) {
	if m.cfg.Ephemeral {
		m.flash = "projects were set with -p and can't be edited here"
		return m, nil
	}
	start := m.cwd
	if _, err := os.Stat(filepath.Join(start, ".git")); err == nil {
		start = filepath.Dir(start)
	}
	in := textinput.New()
	in.Prompt = ""
	in.SetValue(config.ShortHome(start))
	in.CursorEnd()
	m.finder = &finder{input: in, picked: map[string]bool{}}
	return m, m.finder.input.Focus()
}

func (f *finder) free(m *Model) []gitrepo.Found {
	return slices.DeleteFunc(slices.Clone(f.repos), func(r gitrepo.Found) bool {
		return slices.Contains(m.cfg.Projects, r.Dir)
	})
}

func (m Model) handleFinderKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := *m.finder
	if f.loading {
		if k.Type == tea.KeyEsc {
			m.finder = nil
		}
		return m, nil
	}
	if !f.listed {
		switch k.Type {
		case tea.KeyEsc:
			m.finder = nil
			return m, nil
		case tea.KeyEnter:
			root, err := filepath.Abs(config.ExpandHome(strings.TrimSpace(f.input.Value())))
			if err != nil {
				m.flash = err.Error()
				return m, nil
			}
			f.loading, f.root = true, root
			m.finder = &f
			return m, func() tea.Msg {
				repos, err := gitrepo.Find(root)
				return foundMsg{root, repos, err}
			}
		}
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(k)
		m.finder = &f
		return m, cmd
	}

	switch k.String() {
	case "esc":
		f.listed, f.repos, f.picked, f.cursor, f.offset = false, nil, map[string]bool{}, 0, 0
		cmd := f.input.Focus()
		m.finder = &f
		return m, cmd
	case "q":
		m.finder = nil
		return m, nil
	case "j", "down":
		f.cursor = min(f.cursor+1, len(f.repos)-1)
	case "k", "up":
		f.cursor = max(f.cursor-1, 0)
	case " ":
		if f.cursor < len(f.repos) {
			if d := f.repos[f.cursor].Dir; !slices.Contains(m.cfg.Projects, d) {
				if f.picked[d] {
					delete(f.picked, d)
				} else {
					f.picked[d] = true
				}
			}
		}
	case "a":
		free := f.free(&m)
		all := len(f.picked) == len(free)
		f.picked = map[string]bool{}
		if !all {
			for _, r := range free {
				f.picked[r.Dir] = true
			}
		}
	case "enter":
		return m.watchPicked(f)
	}
	vis := max(1, m.h-m.chrome()-2-3)
	f.offset = max(min(f.offset, f.cursor), f.cursor-vis+1)
	m.finder = &f
	return m, nil
}

func (m Model) onFound(msg foundMsg) (tea.Model, tea.Cmd) {
	if m.finder == nil || m.finder.root != msg.root {
		return m, nil // cancelled while searching
	}
	f := *m.finder
	f.loading = false
	if msg.err != nil {
		m.finder = &f
		m.flash = msg.err.Error()
		return m, nil
	}
	f.repos, f.listed = msg.repos, true
	f.input.Blur()
	m.finder = &f
	return m, nil
}

func (m Model) watchPicked(f finder) (tea.Model, tea.Cmd) {
	if len(f.picked) == 0 {
		m.flash = "nothing picked: space picks a repo, a picks them all"
		return m, nil
	}
	// Keep the on-screen order rather than the map's.
	for _, r := range f.repos {
		if f.picked[r.Dir] {
			m.cfg.Projects = append(m.cfg.Projects, r.Dir)
		}
	}
	m.finder = nil
	m.flash = fmt.Sprintf("watching %d more project(s)", len(f.picked))
	if err := m.cfg.Save(); err != nil {
		m.flash = "added, but could not save config: " + err.Error()
	}
	m.openSection(secProjects)
	return m, m.startRefresh()
}

func (m Model) finderLines() []string {
	f := m.finder
	lines := []string{sTitle.Render("Find projects"), ""}
	if !f.listed {
		lines = append(lines, sDim.Render("Folder to look in"), "  "+f.input.View())
		if f.loading {
			lines = append(lines, "", sAccent.Render("  searching…"))
		}
		return lines
	}
	github := 0
	for _, r := range f.repos {
		if r.GitHub {
			github++
		}
	}
	lines = []string{sTitle.Render("Find projects") + sDim.Render(fmt.Sprintf("  %s · %d GitHub, %d other", config.ShortHome(f.root), github, len(f.repos)-github)), ""}
	if len(f.repos) == 0 {
		return append(lines, sDim.Render("  No git repositories here."))
	}
	const nameW, refW = 26, 38
	vis := max(1, m.h-m.chrome()-2-3)
	for i := f.offset; i < min(len(f.repos), f.offset+vis); i++ {
		r := f.repos[i]
		marker := "  "
		if i == f.cursor {
			marker = sAccent.Render("▸ ")
		}
		box := "[ ] "
		switch {
		case slices.Contains(m.cfg.Projects, r.Dir):
			box = sDim.Render(" ✓  ")
		case f.picked[r.Dir]:
			box = sAccent.Render("[x] ")
		}
		ref := sDim.Render("no GitHub remote")
		if r.GitHub {
			ref = r.Ref.String()
		}
		lines = append(lines, marker+box+pad(filepath.Base(r.Dir), nameW-1)+" "+pad(ref, refW-1)+" "+sDim.Render(config.ShortHome(r.Dir)))
	}
	return append(lines, "", sDim.Render(fmt.Sprintf("  %d picked · ✓ already watched", len(f.picked))))
}
