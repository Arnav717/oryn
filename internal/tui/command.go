package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Commands are kept to a word or a letter. Anything else is a package search:
// type a few letters, enter opens the best match.
var commands = map[string]func(m *Model) tea.Cmd{
	"q": func(m *Model) tea.Cmd { return tea.Quit },
	"r": func(m *Model) tea.Cmd { return m.startRefresh() },
	"?": func(m *Model) tea.Cmd { m.showHelp = true; return nil },
	"h": func(m *Model) tea.Cmd { m.screen = screenHome; return nil },
	"p": func(m *Model) tea.Cmd { m.focus = pPortfolio; return nil },
	"v": func(m *Model) tea.Cmd { m.focus = pVulns; return nil },
	"s": func(m *Model) tea.Cmd { m.focus = pStatus; return nil },
	"t": func(m *Model) tea.Cmd { m.focus = pTape; return nil },
}

var aliases = map[string]string{
	"quit": "q", "exit": "q",
	"refresh": "r",
	"help":    "?",
	"home":    "h",
	"port":    "p", "portfolio": "p",
	"c": "v", "cve": "v", "vuln": "v", "vulns": "v",
	"status": "s", "up": "s",
	"tape": "t", "events": "t",
}

func (m Model) runCommand(input string) (tea.Model, tea.Cmd) {
	q := strings.ToLower(strings.TrimSpace(input))
	if q == "" {
		return m, nil
	}
	m.detail = ""
	if a, ok := aliases[q]; ok {
		q = a
	}
	if fn, ok := commands[q]; ok {
		return m, fn(&m)
	}
	// Bloomberg habit: tolerate a trailing function word.
	q = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(q, " des"), " pkg"))
	if keys := m.matches(q, 1); len(keys) > 0 {
		m.openDetail(keys[0])
	} else {
		m.flash = "no package matching " + q
	}
	return m, nil
}

// matches fuzzy-ranks packages against q: exact, prefix, substring, then
// subsequence; ties keep portfolio order (most at-risk first).
func (m *Model) matches(q string, limit int) []string {
	q = strings.ToLower(q)
	if q == "" {
		return nil
	}
	rank := map[string]int{}
	for i, k := range m.pkgOrder {
		rank[k] = i
	}
	type hit struct {
		key   string
		score int
	}
	var hits []hit
	for k, p := range m.pkgs {
		name := strings.ToLower(p.Name)
		short := shortName(name)
		score := -1
		switch {
		case name == q || short == q:
			score = 0
		case strings.HasPrefix(name, q) || strings.HasPrefix(short, q):
			score = 1
		case strings.Contains(name, q):
			score = 2
		case subsequence(name, q):
			score = 3
		}
		if score < 0 {
			continue
		}
		hits = append(hits, hit{k, score})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		ri, iok := rank[hits[i].key]
		rj, jok := rank[hits[j].key]
		if iok && jok {
			return ri < rj
		}
		if iok != jok {
			return iok
		}
		return hits[i].key < hits[j].key
	})
	out := make([]string, 0, min(limit, len(hits)))
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].key)
	}
	return out
}

// shortName is the last path segment, ignoring a Go major-version suffix:
// github.com/go-chi/chi/v5 → chi, @types/node → node.
func shortName(name string) string {
	parts := strings.Split(name, "/")
	last := parts[len(parts)-1]
	if len(parts) > 1 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		last = parts[len(parts)-2]
	}
	return last
}

func subsequence(s, q string) bool {
	i := 0
	for _, r := range s {
		if i < len(q) && byte(r) == q[i] {
			i++
		}
	}
	return i == len(q)
}

// complete replaces the input with the best match's name.
func (m *Model) complete() {
	if keys := m.matches(m.input.Value(), 1); len(keys) > 0 {
		m.input.SetValue(m.pkgs[keys[0]].Name)
		m.input.CursorEnd()
	}
}
