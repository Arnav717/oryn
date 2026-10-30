package agents

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type claudeProc struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	UpdatedAt  int64  `json:"updatedAt"`
}

type claudeJob struct {
	State     string    `json:"state"`
	Tempo     string    `json:"tempo"`
	Needs     string    `json:"needs"`
	Name      string    `json:"name"`
	Tokens    int64     `json:"tokens"`
	Cwd       string    `json:"cwd"`
	SessionID string    `json:"sessionId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// claudeSessions merges running processes (~/.claude/sessions/<pid>.json) with
// background jobs (~/.claude/jobs/*/state.json), which say more but can be
// left behind long after they stopped.
func claudeSessions(home string, now time.Time) []Session {
	root := filepath.Join(home, ".claude")
	byID := map[string]*Session{}
	live := map[string]bool{}

	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*.json"))
	for _, f := range files {
		var p claudeProc
		if !readJSON(f, &p) || !alive(p.PID) {
			continue
		}
		// SDK sessions are other programs driving Claude, not agents you started.
		if p.Entrypoint == "sdk-cli" || p.SessionID == "" {
			continue
		}
		s := byID[p.SessionID]
		if s == nil {
			s = &Session{Tool: "claude", ID: p.SessionID, Dir: p.Cwd, Title: oneLine(p.Name), State: Idle}
			byID[p.SessionID] = s
		}
		live[p.SessionID] = true
		// A foreground window and its background job share a session.
		if procState(p.Status) < s.State {
			s.State = procState(p.Status)
		}
		if t := time.UnixMilli(p.UpdatedAt); t.After(s.Updated) {
			s.Updated = t
		}
		if p.Kind == "bg" {
			s.Title = oneLine(p.Name)
		}
	}

	files, _ = filepath.Glob(filepath.Join(root, "jobs", "*", "state.json"))
	for _, f := range files {
		var j claudeJob
		if !readJSON(f, &j) || j.SessionID == "" {
			continue
		}
		s := byID[j.SessionID]
		if s == nil {
			if now.Sub(j.UpdatedAt) > history {
				continue
			}
			s = &Session{Tool: "claude", ID: j.SessionID, Dir: j.Cwd, State: Idle}
			byID[j.SessionID] = s
		}
		if s.Title == "" {
			s.Title = oneLine(j.Name)
		}
		s.Tokens = j.Tokens
		if j.UpdatedAt.After(s.Updated) {
			s.Updated = j.UpdatedAt
		}
		if st := jobState(j, now); st < s.State || !live[j.SessionID] {
			s.State = st
		}
		if s.State == Waiting {
			s.Needs = oneLine(j.Needs)
		}
	}

	out := make([]Session, 0, len(byID))
	for _, s := range byID {
		out = append(out, *s)
	}
	return out
}

func procState(status string) State {
	switch {
	case status == "busy":
		return Working
	case strings.Contains(status, "wait"), strings.Contains(status, "block"), strings.Contains(status, "permission"):
		return Waiting
	}
	return Idle
}

// jobState reads a job's state file. Only a recent one counts: an old
// "working" job is one that died without saying so.
func jobState(j claudeJob, now time.Time) State {
	switch j.State {
	case "done":
		return Done
	case "failed":
		return Failed
	}
	if now.Sub(j.UpdatedAt) > stale {
		return Idle
	}
	switch {
	case j.Tempo == "blocked", j.Needs != "":
		return Waiting
	case j.Tempo == "active":
		return Working
	}
	return Idle
}

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}
