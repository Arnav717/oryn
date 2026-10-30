// Package store persists the activity timeline in SQLite.
package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

type Event struct {
	Key      string // dedupe key: an event is recorded once
	Kind     string // vuln, release, status
	Severity int
	Entity   string
	Title    string
	URL      string
	At       time.Time
}

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS events (
	id       INTEGER PRIMARY KEY,
	key      TEXT NOT NULL UNIQUE,
	kind     TEXT NOT NULL,
	severity INTEGER NOT NULL DEFAULT 0,
	entity   TEXT NOT NULL,
	title    TEXT NOT NULL,
	url      TEXT NOT NULL DEFAULT '',
	ts       INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS claims (
	project TEXT NOT NULL,
	path    TEXT NOT NULL,
	agent   TEXT NOT NULL,
	expires INTEGER NOT NULL,
	PRIMARY KEY (project, path)
);
CREATE TABLE IF NOT EXISTS hidden (
	key   TEXT PRIMARY KEY,
	until INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS events_ts ON events(ts);
CREATE INDEX IF NOT EXISTS events_entity ON events(kind, entity, ts);
`

func DefaultPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "oryn.db"
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "oryn", "oryn.db")
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Record inserts e unless an event with the same key exists. It reports
// whether the event was new.
func (s *Store) Record(e Event) (bool, error) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO events (key, kind, severity, entity, title, url, ts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Key, e.Kind, e.Severity, e.Entity, e.Title, e.URL, e.At.Unix())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// HasEvents reports whether anything has been recorded yet.
func (s *Store) HasEvents() (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM events)`).Scan(&n)
	return n == 1, err
}

// LastTitle returns the title of the newest event of kind for entity.
func (s *Store) LastTitle(kind, entity string) (string, error) {
	var t string
	err := s.db.QueryRow(
		`SELECT title FROM events WHERE kind = ? AND entity = ? ORDER BY ts DESC, id DESC LIMIT 1`,
		kind, entity).Scan(&t)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return t, err
}

func (s *Store) Recent(limit int) ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT key, kind, severity, entity, title, url, ts FROM events ORDER BY ts DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.Key, &e.Kind, &e.Severity, &e.Entity, &e.Title, &e.URL, &ts); err != nil {
			return nil, err
		}
		e.At = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Meta returns a stored value, or "" when unset.
func (s *Store) Meta(k string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM meta WHERE k = ?`, k).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetMeta(k, v string) error {
	_, err := s.db.Exec(`INSERT INTO meta (k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, k, v)
	return err
}

// TouchLastSeen returns when the user last opened the UI (zero on first run)
// and marks now as the latest visit.
func (s *Store) TouchLastSeen(now time.Time) (time.Time, error) {
	v, err := s.Meta("last_seen")
	if err != nil {
		return time.Time{}, err
	}
	var prev time.Time
	if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
		prev = time.Unix(n, 0)
	}
	return prev, s.SetMeta("last_seen", strconv.FormatInt(now.Unix(), 10))
}

// Since returns events after t (repo baselines excluded), most severe first,
// then newest first.
func (s *Store) Since(after time.Time) ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT key, kind, severity, entity, title, url, ts FROM events
		 WHERE ts > ? AND kind != 'repo' ORDER BY severity DESC, ts DESC, id DESC LIMIT 2000`, after.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.Key, &e.Kind, &e.Severity, &e.Entity, &e.Title, &e.URL, &ts); err != nil {
			return nil, err
		}
		e.At = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}
