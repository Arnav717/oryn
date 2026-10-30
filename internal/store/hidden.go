package store

import "time"

// Hide keeps a Needs you item out of the list until then.
func (s *Store) Hide(key string, until time.Time) error {
	_, err := s.db.Exec(`INSERT INTO hidden (key, until) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET until = excluded.until`, key, until.Unix())
	return err
}

// Unhide brings items back.
func (s *Store) Unhide(keys []string) error {
	for _, k := range keys {
		if _, err := s.db.Exec(`DELETE FROM hidden WHERE key = ?`, k); err != nil {
			return err
		}
	}
	return nil
}

// Hidden lists what's still hidden, dropping whatever has run out.
func (s *Store) Hidden(now time.Time) (map[string]time.Time, error) {
	if _, err := s.db.Exec(`DELETE FROM hidden WHERE until <= ?`, now.Unix()); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT key, until FROM hidden`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var k string
		var until int64
		if err := rows.Scan(&k, &until); err != nil {
			return nil, err
		}
		out[k] = time.Unix(until, 0)
	}
	return out, rows.Err()
}
