package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func repo(t *testing.T, dir, remote string) {
	t.Helper()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	if remote != "" {
		if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", remote).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "zeta"), "git@github.com:me/zeta.git")
	repo(t, filepath.Join(root, "group", "alpha"), "https://github.com/me/alpha")
	repo(t, filepath.Join(root, "local"), "")
	repo(t, filepath.Join(root, "gitlab"), "git@gitlab.com:me/x.git")
	repo(t, filepath.Join(root, "zeta", "nested"), "git@github.com:me/nested.git")     // inside a repo
	repo(t, filepath.Join(root, "app", "node_modules", "dep"), "git@github.com:o/dep") // a dependency
	repo(t, filepath.Join(root, ".cache", "hidden"), "git@github.com:o/hidden")        // hidden
	repo(t, filepath.Join(root, "a", "b", "c", "d", "deep"), "git@github.com:o/deep")  // too deep
	if err := os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, "worktree")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name   string
		github bool
	}{{"alpha", true}, {"zeta", true}, {"gitlab", false}, {"local", false}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		if filepath.Base(got[i].Dir) != w.name || got[i].GitHub != w.github {
			t.Errorf("%d: %+v, want %+v", i, got[i], w)
		}
	}
	if got[1].Ref.String() != "me/zeta" {
		t.Errorf("ref: %s", got[1].Ref)
	}
}

func TestFindRootIsARepo(t *testing.T) {
	root := t.TempDir()
	repo(t, root, "git@github.com:me/solo.git")
	if got, err := Find(root); err != nil || len(got) != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestFindMissingFolder(t *testing.T) {
	if _, err := Find(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected an error")
	}
}
