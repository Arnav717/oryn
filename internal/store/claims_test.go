package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestClaims(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "o.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()

	if c, err := s.Claim("/p", "a", []string{"src/auth", "go.mod"}, now); err != nil || len(c) != 0 {
		t.Fatalf("first claim: %v %v", c, err)
	}
	// Same file, a file inside a claimed folder, and a parent of one all clash.
	for _, p := range []string{"go.mod", "src/auth/login.go", "src", "."} {
		c, err := s.Claim("/p", "b", []string{"free.txt", p}, now)
		if err != nil || len(c) == 0 {
			t.Errorf("%s should clash: %v %v", p, c, err)
		}
	}
	// A clash takes nothing, not even the free path.
	held, _ := s.Claims("/p", now)
	if len(held) != 2 {
		t.Errorf("all-or-none broken: %+v", held)
	}
	if c, _ := s.Claim("/p", "b", []string{"src/authx"}, now); len(c) != 0 {
		t.Errorf("similar name shouldn't clash: %+v", c)
	}
	if c, _ := s.Claim("/p", "a", []string{"src/auth"}, now); len(c) != 0 {
		t.Errorf("reclaiming your own: %+v", c)
	}
	if c, _ := s.Claim("/other", "b", []string{"go.mod"}, now); len(c) != 0 {
		t.Errorf("projects are separate: %+v", c)
	}

	if err := s.Release("/p", "b", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Release("/p", "a", []string{"go.mod"}); err != nil {
		t.Fatal(err)
	}
	if held, _ = s.Claims("/p", now); len(held) != 1 || held[0].Path != "src/auth" {
		t.Errorf("after release: %+v", held)
	}

	// Expired claims stop counting.
	later := now.Add(ClaimTTL + time.Minute)
	if held, _ = s.Claims("/p", later); len(held) != 0 {
		t.Errorf("expired: %+v", held)
	}
	if c, _ := s.Claim("/p", "b", []string{"src/auth"}, later); len(c) != 0 {
		t.Errorf("expired claim blocked: %+v", c)
	}
}
