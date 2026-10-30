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

func TestHideExpires(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "o.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	s.Hide("soon", now.Add(time.Hour))
	s.Hide("later", now.Add(48*time.Hour))
	s.Hide("gone", now.Add(48*time.Hour))
	s.Unhide([]string{"gone"})

	got, err := s.Hidden(now)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, err %v", got, err)
	}
	if got, _ = s.Hidden(now.Add(2 * time.Hour)); len(got) != 1 || got["later"].IsZero() {
		t.Errorf("after the first ran out: %v", got)
	}
}
