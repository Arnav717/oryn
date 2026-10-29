package app

import (
	"path/filepath"
	"testing"

	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
)

func TestGitHubEvents(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snap := func(mine ...source.PR) *Snapshot {
		return &Snapshot{GitHub: &source.GitHubData{
			Login:  "me",
			Review: []source.PR{{Repo: "o/r", Number: 1, Title: "add x", Author: "bob", URL: "https://github.com/o/r/pull/1"}},
			Mine:   mine,
		}}
	}
	failing := source.PR{Repo: "o/r", Number: 2, Title: "fix y", CI: "FAILURE", CommitOID: "aaaaaaa1111"}

	s := snap(failing, source.PR{Repo: "o/r", Number: 3, CI: "SUCCESS", CommitOID: "bbbbbbb"})
	if err := recordEvents(st, s); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range s.New {
		kinds[e.Kind]++
	}
	if kinds["review"] != 1 || kinds["ci"] != 1 || len(s.New) != 2 {
		t.Fatalf("first pass events: %v", kinds)
	}

	// Same state again: nothing new, so the daemon stays quiet.
	s = snap(failing)
	if err := recordEvents(st, s); err != nil || len(s.New) != 0 {
		t.Fatalf("second pass: err=%v new=%d", err, len(s.New))
	}

	// A new commit that fails again is a fresh alert.
	failing.CommitOID = "ccccccc2222"
	s = snap(failing)
	if err := recordEvents(st, s); err != nil || len(s.New) != 1 || s.New[0].Kind != "ci" {
		t.Fatalf("new commit: err=%v new=%+v", err, s.New)
	}
}
