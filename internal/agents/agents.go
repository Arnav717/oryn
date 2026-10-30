// Package agents reads what AI coding agents on this machine are doing. Each
// tool gets a small adapter over its own local files. Those files are private
// formats that can change, so an adapter that can't read them yields nothing
// rather than an error, and nothing here ever writes to them.
package agents

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type State int

// Ordered by how much they need you.
const (
	Waiting State = iota
	Working
	Failed
	Idle
	Done
)

func (s State) String() string {
	return [...]string{"waiting", "working", "failed", "idle", "done"}[s]
}

// Session is one agent conversation.
type Session struct {
	Tool    string // claude, codex, opencode, agy
	ID      string
	Dir     string
	Title   string
	State   State
	Needs   string // what it's waiting on, when the tool says
	Tokens  int64
	Updated time.Time
}

// Active reports whether it's running or waiting on you right now.
func (s Session) Active() bool { return s.State == Waiting || s.State == Working }

// Resume is the command that reopens the session.
func (s Session) Resume() string {
	switch s.Tool {
	case "claude":
		return "claude --resume " + s.ID
	case "codex":
		return "codex resume " + s.ID
	case "opencode":
		return "opencode --session " + s.ID
	case "agy":
		return "agy --conversation " + s.ID
	}
	return ""
}

const (
	history = 7 * 24 * time.Hour // how far back sessions are listed
	stale   = 24 * time.Hour     // "waiting" older than this has been abandoned
	writing = 2 * time.Minute    // a file touched this recently is being worked on
)

// List returns sessions from every tool, most urgent first.
func List(now time.Time) []Session {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return list(home, now, runningDirs())
}

func list(home string, now time.Time, live liveDirs) []Session {
	var out []Session
	out = append(out, claudeSessions(home, now)...)
	out = append(out, codexSessions(home, now, live)...)
	out = append(out, opencodeSessions(home, now)...)
	out = append(out, agySessions(home, now)...)
	slices.SortStableFunc(out, func(a, b Session) int {
		if c := cmp.Compare(a.State, b.State); c != 0 {
			return c
		}
		return b.Updated.Compare(a.Updated)
	})
	return out
}

// liveDirs is, per tool, the directories it's running in.
type liveDirs map[string]map[string]bool

// runningDirs scans /proc. Elsewhere it finds nothing, and the adapters fall
// back on how recently the tool wrote its files.
func runningDirs() liveDirs {
	live := liveDirs{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return live
	}
	tools := []string{"codex"}
	for _, e := range entries {
		if e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		b, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if err != nil {
			continue
		}
		// Node and Bun launchers put the tool in the second argument.
		args := strings.Split(string(b), "\x00")
		for i := 0; i < min(2, len(args)); i++ {
			tool := filepath.Base(args[i])
			if !slices.Contains(tools, tool) {
				continue
			}
			if cwd, err := os.Readlink(filepath.Join(dir, "cwd")); err == nil {
				if live[tool] == nil {
					live[tool] = map[string]bool{}
				}
				live[tool][cwd] = true
			}
			break
		}
	}
	return live
}

// runState is for a tool that keeps no run status: it's working if it wrote
// just now. A process in the folder alone doesn't count, that's an open
// session waiting for a prompt.
func runState(updated, now time.Time) State {
	if now.Sub(updated) < writing {
		return Working
	}
	return Idle
}

func oneLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}
