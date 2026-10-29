package app

import (
	"path/filepath"
	"testing"

	"github.com/Arnav717/oryn/internal/gitrepo"
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

func TestRepoEventsBaseline(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	repo := func(issues ...source.Issue) *Snapshot {
		return &Snapshot{
			GitHub: &source.GitHubData{Login: "me", Review: []source.PR{{Repo: "o/r", Number: 5}}},
			Repos: map[string]*source.RepoData{"/p": {
				Ref: gitrepo.Ref{Owner: "o", Name: "r"}, Viewer: "me", URL: "https://github.com/o/r", DefaultBranch: "main",
				DefaultCI: "FAILURE", Commits: []source.Commit{{SHA: "abcdef123456"}},
				Issues: issues,
				PRs: []source.PR{
					{Repo: "o/r", Number: 4, Author: "bob"}, // someone else's PR
					{Repo: "o/r", Number: 5, Author: "bob"}, // already a review request
					{Repo: "o/r", Number: 6, Author: "me"},  // your own
				},
			}},
		}
	}
	// "review" events come from the account inbox, not the repo; ignore them here.
	repoKinds := func(s *Snapshot) map[string]int {
		m := map[string]int{}
		for _, e := range s.New {
			if e.Kind != "review" {
				m[e.Kind]++
			}
		}
		return m
	}

	// First sight of a repo is a silent baseline: stored, but not "new".
	s := repo(source.Issue{Number: 1, Author: "bob"}, source.Issue{Number: 2, Author: "me"})
	if err := recordEvents(st, s); err != nil {
		t.Fatal(err)
	}
	if k := repoKinds(s); len(k) != 0 {
		t.Fatalf("baseline should not alert, got %v", k)
	}

	// Later: a new issue from someone else is an alert; your own issue is not, and
	// neither are PRs that are yours or already review requests.
	s = repo(source.Issue{Number: 1, Author: "bob"}, source.Issue{Number: 2, Author: "me"},
		source.Issue{Number: 3, Author: "carol"}, source.Issue{Number: 4, Author: "me"})
	if err := recordEvents(st, s); err != nil {
		t.Fatal(err)
	}
	if k := repoKinds(s); k["issue"] != 1 || len(k) != 1 {
		t.Errorf("want exactly one new issue event, got %v", k)
	}
}
