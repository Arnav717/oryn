package agents

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func job(t *testing.T, home, id, state, tempo, needs string, age time.Duration) {
	t.Helper()
	write(t, filepath.Join(home, ".claude/jobs", id, "state.json"), fmt.Sprintf(
		`{"state":%q,"tempo":%q,"needs":%q,"name":"job %s","tokens":5,"cwd":"/p","sessionId":%q,"updatedAt":%q,"detail":"SECRET PROMPT"}`,
		state, tempo, needs, id, id, now.Add(-age).Format(time.RFC3339)))
}

func find(t *testing.T, ss []Session, id string) Session {
	t.Helper()
	for _, s := range ss {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no session %s in %+v", id, ss)
	return Session{}
}

func TestClaudeJobs(t *testing.T) {
	home := t.TempDir()
	job(t, home, "wait", "blocked", "blocked", "approve deploy\nmore", time.Hour)
	job(t, home, "work", "blocked", "active", "", time.Minute)
	job(t, home, "fin", "done", "idle", "", time.Hour)
	job(t, home, "bad", "failed", "idle", "", time.Hour)
	job(t, home, "abandoned", "working", "blocked", "stale ask", 3*24*time.Hour)
	job(t, home, "ancient", "working", "active", "", 30*24*time.Hour)

	ss := claudeSessions(home, now)
	if s := find(t, ss, "wait"); s.State != Waiting || s.Needs != "approve deploy" || s.Title != "job wait" || s.Tokens != 5 {
		t.Errorf("wait: %+v", s)
	}
	for id, want := range map[string]State{"work": Working, "fin": Done, "bad": Failed, "abandoned": Idle} {
		if s := find(t, ss, id); s.State != want {
			t.Errorf("%s: %v, want %v", id, s.State, want)
		}
	}
	if s := find(t, ss, "abandoned"); s.Needs != "" {
		t.Errorf("an abandoned job isn't asking for anything: %q", s.Needs)
	}
	for _, s := range ss {
		if s.ID == "ancient" {
			t.Error("old jobs should drop off")
		}
		if strings.Contains(fmt.Sprintf("%+v", s), "SECRET") {
			t.Error("the job's detail text must not be read")
		}
	}
}

func TestClaudeLiveProcess(t *testing.T) {
	home := t.TempDir()
	me := os.Getpid()
	session := func(file, id, kind, entry, status, name string, pid int) {
		write(t, filepath.Join(home, ".claude/sessions", file+".json"), fmt.Sprintf(
			`{"pid":%d,"sessionId":%q,"cwd":"/p","kind":%q,"entrypoint":%q,"status":%q,"name":%q,"updatedAt":%d}`,
			pid, id, kind, entry, status, name, now.UnixMilli()))
	}
	session("a", "shared", "interactive", "cli", "idle", "window", me)
	session("b", "shared", "bg", "cli", "busy", "the real name", me)
	session("c", "observer", "interactive", "sdk-cli", "idle", "plugin", me)
	session("d", "dead", "interactive", "cli", "busy", "gone", 1<<30)

	ss := claudeSessions(home, now)
	if len(ss) != 1 {
		t.Fatalf("got %+v", ss)
	}
	if s := ss[0]; s.ID != "shared" || s.State != Working || s.Title != "the real name" {
		t.Errorf("merged: %+v", s)
	}

	// An old job can't drag a session that's live and busy back to idle.
	job(t, home, "shared", "working", "idle", "", 48*time.Hour)
	if s := find(t, claudeSessions(home, now), "shared"); s.State != Working {
		t.Errorf("live busy session: %v", s.State)
	}
}

func rollout(t *testing.T, home, name string, age time.Duration, lines ...string) {
	t.Helper()
	p := filepath.Join(home, ".codex/sessions/2026/10/30", name+".jsonl")
	write(t, p, strings.Join(lines, "\n")+"\n")
	if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
		t.Fatal(err)
	}
}

func TestCodex(t *testing.T) {
	home := t.TempDir()
	meta := func(id string) string {
		return fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":"/c"}}`, id)
	}
	user := `{"type":"event_msg","payload":{"type":"user_message","message":"fix the login\nplease"}}`
	start := `{"type":"event_msg","payload":{"type":"task_started"}}`
	end := `{"type":"event_msg","payload":{"type":"task_complete"}}`
	rollout(t, home, "busy", time.Minute, meta("busy"), user, start)
	rollout(t, home, "fin", time.Minute, meta("fin"), user, start, end)
	rollout(t, home, "hung", time.Hour, meta("hung"), user, start)
	rollout(t, home, "liveopen", time.Hour, meta("liveopen"), `{"type":"session_meta","payload":{"id":"x"}}`, start)
	rollout(t, home, "old", 30*24*time.Hour, meta("old"), user, start)

	ss := codexSessions(home, now, liveDirs{})
	if s := find(t, ss, "busy"); s.State != Working || s.Title != "fix the login" || s.Dir != "/c" {
		t.Errorf("busy: %+v", s)
	}
	if s := find(t, ss, "fin"); s.State != Idle {
		t.Errorf("fin: %v", s.State)
	}
	if s := find(t, ss, "hung"); s.State != Idle {
		t.Errorf("a turn left open by a dead process: %v", s.State)
	}
	if s := find(t, codexSessions(home, now, liveDirs{"codex": {"/c": true}}), "hung"); s.State != Working {
		t.Errorf("open turn with codex running there: %v", s.State)
	}
	if len(ss) != 4 {
		t.Errorf("old rollouts should drop off: %d", len(ss))
	}
}

func TestOpenCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	p := filepath.Join(home, "data/opencode/opencode.db")
	write(t, p, "")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE session (id TEXT, directory TEXT, title TEXT, parent_id TEXT, time_updated INT, time_archived INT, tokens_input INT, tokens_output INT)`); err != nil {
		t.Fatal(err)
	}
	insert := func(id string, parent any, age time.Duration, archived any) {
		if _, err := db.Exec(`INSERT INTO session VALUES (?,?,?,?,?,?,?,?)`, id, "/o", "title "+id, parent, now.Add(-age).UnixMilli(), archived, 10, 5); err != nil {
			t.Fatal(err)
		}
	}
	insert("now", nil, time.Second, nil)
	insert("earlier", nil, time.Hour, nil)
	insert("child", "now", time.Second, nil)
	insert("gone", nil, time.Second, 1)
	insert("old", nil, 30*24*time.Hour, nil)

	ss := opencodeSessions(home, now)
	if len(ss) != 2 {
		t.Fatalf("got %+v", ss)
	}
	if s := find(t, ss, "now"); s.State != Working || s.Tokens != 15 || s.Title != "title now" {
		t.Errorf("now: %+v", s)
	}
	if s := find(t, ss, "earlier"); s.State != Idle {
		t.Errorf("earlier: %v", s.State)
	}
}

func TestAgy(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".gemini/antigravity-cli/conversation_summaries.db")
	write(t, p, "")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE conversation_summaries (conversation_id TEXT, title TEXT, status TEXT, last_modified_time TEXT, workspace_uris TEXT, parent_conversation_id TEXT, killed INT, not_fully_idle INT)`); err != nil {
		t.Fatal(err)
	}
	insert := func(id, status string, age time.Duration, parent string, killed, busy int) {
		_, err := db.Exec(`INSERT INTO conversation_summaries VALUES (?,?,?,?,?,?,?,?)`, id, "t "+id, status,
			now.Add(-age).UTC().Format("2006-01-02 15:04:05.999999999-07:00"), `["file:///data/my%20app"]`, parent, killed, busy)
		if err != nil {
			t.Fatal(err)
		}
	}
	const idle = "CASCADE_RUN_STATUS_IDLE"
	insert("idle", idle, time.Hour, "", 0, 0)
	insert("run", "CASCADE_RUN_STATUS_RUNNING", time.Minute, "", 0, 0)
	insert("wait", "CASCADE_RUN_STATUS_WAITING_FOR_USER", time.Hour, "", 0, 0)
	insert("crashed", "CASCADE_RUN_STATUS_RUNNING", 3*24*time.Hour, "", 0, 0)
	insert("dead", idle, time.Hour, "", 1, 0)
	insert("sub", idle, time.Hour, "idle", 0, 0)
	insert("old", idle, 30*24*time.Hour, "", 0, 0)

	ss := agySessions(home, now)
	if len(ss) != 5 {
		t.Fatalf("got %+v", ss)
	}
	for id, want := range map[string]State{"idle": Idle, "run": Working, "wait": Waiting, "crashed": Idle, "dead": Failed} {
		if s := find(t, ss, id); s.State != want {
			t.Errorf("%s: %v, want %v", id, s.State, want)
		}
	}
	if s := find(t, ss, "idle"); s.Dir != "/data/my app" || s.Tool != "agy" {
		t.Errorf("idle: %+v", s)
	}
}

func TestAgyTime(t *testing.T) {
	for _, s := range []string{"2026-10-29 15:50:34.001374825+00:00", "2026-10-29T15:50:34.001374825Z"} {
		if got, ok := agyTime(s); !ok || got.Hour() != 15 {
			t.Errorf("%s: %v %v", s, got, ok)
		}
	}
	if _, ok := agyTime("yesterday"); ok {
		t.Error("garbage parsed")
	}
}

func TestOldSchemaYieldsNothing(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gemini/antigravity-cli/conversation_summaries.db"), "not a database")
	write(t, filepath.Join(home, ".claude/jobs/x/state.json"), "{broken")
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "nope"))
	if got := list(home, now, liveDirs{}); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestListOrder(t *testing.T) {
	home := t.TempDir()
	job(t, home, "a-idle", "blocked", "idle", "", time.Minute)
	job(t, home, "b-work", "working", "active", "", time.Minute)
	job(t, home, "c-wait", "blocked", "blocked", "", time.Hour)
	ss := list(home, now, liveDirs{})
	if ss[0].ID != "c-wait" || ss[1].ID != "b-work" {
		t.Errorf("order: %+v", ss)
	}
}

func TestResume(t *testing.T) {
	for tool, want := range map[string]string{"claude": "claude --resume i", "codex": "codex resume i", "opencode": "opencode --session i", "agy": "agy --conversation i"} {
		if got := (Session{Tool: tool, ID: "i"}).Resume(); got != want {
			t.Errorf("%s: %q", tool, got)
		}
	}
}
