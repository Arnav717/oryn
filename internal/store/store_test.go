package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTouchLastSeen(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Unix(1_700_000_000, 0)
	prev, err := s.TouchLastSeen(now)
	if err != nil || !prev.IsZero() {
		t.Fatalf("first run: prev=%v err=%v", prev, err)
	}
	prev, err = s.TouchLastSeen(now.Add(time.Hour))
	if err != nil || !prev.Equal(now) {
		t.Fatalf("second run: prev=%v err=%v", prev, err)
	}
}
