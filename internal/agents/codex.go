package agents

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	headBytes = 1 << 20
	tailBytes = 256 << 10
)

type codexLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Cwd     string `json:"cwd"`
		Message string `json:"message"`
	} `json:"payload"`
}

// codexSessions reads rollout logs: the first line has the session's folder,
// the first user message is its title, and the last turn event says whether a
// turn is open. Codex logs no approval requests, so it never shows as waiting.
func codexSessions(home string, now time.Time, live liveDirs) []Session {
	files, _ := filepath.Glob(filepath.Join(home, ".codex", "sessions", "*", "*", "*", "*.jsonl"))
	var out []Session
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil || now.Sub(fi.ModTime()) > history {
			continue
		}
		s := Session{Tool: "codex", Updated: fi.ModTime(), State: Idle}
		turnOpen := false
		eachLine(f, 0, headBytes, func(l codexLine) {
			switch {
			case l.Type == "session_meta" && s.ID == "":
				s.ID, s.Dir = l.Payload.ID, l.Payload.Cwd
			case l.Type == "event_msg" && l.Payload.Type == "user_message" && s.Title == "":
				s.Title = oneLine(l.Payload.Message)
			}
		})
		if s.ID == "" {
			continue
		}
		eachLine(f, max(0, fi.Size()-tailBytes), tailBytes, func(l codexLine) {
			if l.Type != "event_msg" {
				return
			}
			switch l.Payload.Type {
			case "task_started":
				turnOpen = true
			case "task_complete", "turn_aborted":
				turnOpen = false
			}
		})
		if turnOpen && (live["codex"][s.Dir] || now.Sub(s.Updated) < writing) {
			s.State = Working
		}
		out = append(out, s)
	}
	return out
}

// eachLine decodes the JSON lines in a byte range of a file, skipping any
// that are cut off by the range.
func eachLine(path string, off, n int64, fn func(codexLine)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.NewSectionReader(f, off, n))
	if err != nil {
		return
	}
	for _, line := range bytes.Split(b, []byte("\n")) {
		var l codexLine
		if json.Unmarshal(line, &l) == nil {
			fn(l)
		}
	}
}
