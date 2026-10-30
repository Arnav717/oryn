package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ClaimTTL is how long a claim lasts: an agent that dies shouldn't hold files
// forever.
const ClaimTTL = 2 * time.Hour

// Claim is an agent saying it's working on a path.
type Claim struct {
	Path    string    `json:"path"`
	Agent   string    `json:"agent"`
	Expires time.Time `json:"expires"`
}

// overlaps: a claim on a folder covers everything inside it.
func overlaps(a, b string) bool {
	a, b = strings.TrimSuffix(a, "/"), strings.TrimSuffix(b, "/")
	return a == b || a == "." || b == "." || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// Claim takes paths for agent, all or none. It returns other agents' claims in
// the way; when there are any, nothing was taken.
func (s *Store) Claim(project, agent string, paths []string, now time.Time) ([]Claim, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	held, err := claimsIn(tx, project, now)
	if err != nil {
		return nil, err
	}
	var conflicts []Claim
	for _, c := range held {
		if c.Agent == agent {
			continue
		}
		for _, p := range paths {
			if overlaps(c.Path, p) {
				conflicts = append(conflicts, c)
				break
			}
		}
	}
	if len(conflicts) > 0 {
		return conflicts, nil
	}
	exp := now.Add(ClaimTTL).Unix()
	for _, p := range paths {
		if _, err := tx.Exec(`INSERT INTO claims (project, path, agent, expires) VALUES (?,?,?,?)
			ON CONFLICT (project, path) DO UPDATE SET agent = excluded.agent, expires = excluded.expires`, project, p, agent, exp); err != nil {
			return nil, err
		}
	}
	return nil, tx.Commit()
}

// Release drops agent's claims on paths, or all of them when paths is empty.
func (s *Store) Release(project, agent string, paths []string) error {
	if len(paths) == 0 {
		_, err := s.db.Exec(`DELETE FROM claims WHERE project = ? AND agent = ?`, project, agent)
		return err
	}
	for _, p := range paths {
		if _, err := s.db.Exec(`DELETE FROM claims WHERE project = ? AND agent = ? AND path = ?`, project, agent, p); err != nil {
			return err
		}
	}
	return nil
}

// Claims lists a project's live claims.
func (s *Store) Claims(project string, now time.Time) ([]Claim, error) {
	return claimsIn(s.db, project, now)
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func claimsIn(q querier, project string, now time.Time) ([]Claim, error) {
	rows, err := q.Query(`SELECT path, agent, expires FROM claims WHERE project = ? AND expires > ? ORDER BY path`, project, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Claim
	for rows.Next() {
		var c Claim
		var exp int64
		if err := rows.Scan(&c.Path, &c.Agent, &exp); err != nil {
			return nil, err
		}
		c.Expires = time.Unix(exp, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}
