package agents

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the driver, also used by internal/store
)

// openRO opens another tool's database without being able to change it.
func openRO(path string) (*sql.DB, bool) {
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: "mode=ro&_pragma=busy_timeout(500)"}).String())
	return db, err == nil
}

func opencodeSessions(home string, now time.Time) []Session {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		dir = filepath.Join(home, ".local", "share")
	}
	db, ok := openRO(filepath.Join(dir, "opencode", "opencode.db"))
	if !ok {
		return nil
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, directory, title, time_updated, tokens_input + tokens_output FROM session
		WHERE parent_id IS NULL AND time_archived IS NULL AND time_updated > ? ORDER BY time_updated DESC`,
		now.Add(-history).UnixMilli())
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		s := Session{Tool: "opencode"}
		var updated int64
		var tokens sql.NullInt64
		if rows.Scan(&s.ID, &s.Dir, &s.Title, &updated, &tokens) != nil {
			continue
		}
		s.Updated, s.Tokens, s.Title = time.UnixMilli(updated), tokens.Int64, oneLine(s.Title)
		s.State = runState(s.Updated, now)
		out = append(out, s)
	}
	return out
}

// agySessions reads the Antigravity CLI's conversation index.
func agySessions(home string, now time.Time) []Session {
	db, ok := openRO(filepath.Join(home, ".gemini", "antigravity-cli", "conversation_summaries.db"))
	if !ok {
		return nil
	}
	defer db.Close()
	rows, err := db.Query(`SELECT conversation_id, title, status, last_modified_time, workspace_uris, killed, not_fully_idle
		FROM conversation_summaries WHERE parent_conversation_id = '' ORDER BY last_modified_time DESC LIMIT 200`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		s := Session{Tool: "agy"}
		var status, modified, uris string
		var killed, busy int
		if rows.Scan(&s.ID, &s.Title, &status, &modified, &uris, &killed, &busy) != nil {
			continue
		}
		t, ok := agyTime(modified)
		if !ok || now.Sub(t) > history {
			continue
		}
		s.Updated, s.Title, s.Dir = t, oneLine(s.Title), firstFolder(uris)
		s.State = agyState(status, killed != 0, busy != 0, t, now)
		out = append(out, s)
	}
	return out
}

// agyTime accepts the column as stored and as the driver reformats it when the
// column is declared a timestamp.
func agyTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// agyState maps Antigravity's run status. A status can outlive a crash, so
// anything not idle also has to be recent.
func agyState(status string, killed, busy bool, updated, now time.Time) State {
	switch {
	case killed:
		return Failed
	case now.Sub(updated) > stale:
		return Idle
	case strings.Contains(status, "WAIT"), strings.Contains(status, "BLOCK"):
		return Waiting
	case strings.Contains(status, "RUNNING"), busy:
		return Working
	}
	return runState(updated, now)
}

func firstFolder(uris string) string {
	var list []string
	if json.Unmarshal([]byte(uris), &list) != nil || len(list) == 0 {
		return ""
	}
	u, err := url.Parse(list[0])
	if err != nil {
		return ""
	}
	return u.Path
}
