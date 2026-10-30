package agents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitRepo makes a repo with one commit.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "myapp")
	for _, args := range [][]string{{"init", "-q", dir}, {"-C", dir, "config", "user.email", "t@t"}, {"-C", dir, "config", "user.name", "t"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	write(t, filepath.Join(dir, "a.txt"), "x")
	for _, args := range [][]string{{"-C", dir, "add", "."}, {"-C", dir, "commit", "-qm", "first"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return dir
}

// fakeTool stands in for an agent: it records its arguments and stays up.
func fakeTool(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	old, oldSock, oldState := Tools, socket, stateDir
	socket = fmt.Sprintf("oryn-test-%d", os.Getpid())
	stateDir = t.TempDir()
	Tools = append(Tools[:len(Tools):len(Tools)], Tool{"fake", "sh", func(task string) []string { return []string{"-c", script, "fake", task} }})
	t.Cleanup(func() {
		tmux("kill-server").Run()
		Tools, socket, stateDir = old, oldSock, oldState
	})
}

func branches(t *testing.T, repo string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", "oryn/*").CombinedOutput()
	return string(out)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for range 100 {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStartRunsInWorktreeWithoutAShell(t *testing.T) {
	fakeTool(t, `printf '%s' "$1" > task.txt; pwd > where.txt; sleep 30`)
	repo := gitRepo(t)
	marker := filepath.Join(t.TempDir(), "PWNED")
	task := fmt.Sprintf(`fix it's login; touch %s $(touch %s)`, marker, marker)

	r, err := Start(repo, "fake", task, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Branch, "oryn/fix-it-s-login") || r.Slug == "" {
		t.Errorf("run: %+v", r)
	}
	got := filepath.Join(r.Worktree, "task.txt")
	waitFor(t, "the tool to start", func() bool { b, _ := os.ReadFile(got); return len(b) > 0 })
	if b, _ := os.ReadFile(got); string(b) != task {
		t.Errorf("task arrived as %q", b)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the task was run by a shell")
	}
	if !strings.Contains(branches(t, repo), r.Branch) {
		t.Errorf("no branch: %s", branches(t, repo))
	}
	if !panes()[r.Slug] {
		t.Error("session should be running")
	}

	ss := merge(nil, loadRuns(), panes(), nil, now)
	if len(ss) != 1 || ss[0].Run == nil || !ss[0].Running || ss[0].State != Working || ss[0].Project != repo || ss[0].Title != task {
		t.Fatalf("merged: %+v", ss)
	}

	if err := Stop(*ss[0].Run); err != nil {
		t.Fatal(err)
	}
	if panes()[r.Slug] {
		t.Error("still running after Stop")
	}
	if _, err := os.Stat(got); err != nil {
		t.Error("Stop must keep the worktree")
	}
	if !strings.Contains(branches(t, repo), r.Branch) {
		t.Error("Stop must keep the branch")
	}
}

func TestFinishedAgentStaysReadable(t *testing.T) {
	fakeTool(t, `echo done`)
	r, err := Start(gitRepo(t), "fake", "quick job", now)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "it to finish", func() bool { a, ok := panes()[r.Slug]; return ok && !a })
	ss := merge(nil, loadRuns(), panes(), nil, now)
	if ss[0].Running || ss[0].State != Idle {
		t.Errorf("finished: %+v", ss[0])
	}
}

func TestNamesDontCollide(t *testing.T) {
	fakeTool(t, `sleep 30`)
	repo := gitRepo(t)
	a, err := Start(repo, "fake", "same task", now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Start(repo, "fake", "same task", now)
	if err != nil {
		t.Fatal(err)
	}
	if a.Slug == b.Slug || a.Worktree == b.Worktree {
		t.Errorf("collision: %+v %+v", a, b)
	}
}

func TestSubfolderProjectRunsInSameSubfolder(t *testing.T) {
	fakeTool(t, `pwd > where.txt; sleep 30`)
	repo := gitRepo(t)
	write(t, filepath.Join(repo, "pkg/web/b.txt"), "y")
	for _, args := range [][]string{{"-C", repo, "add", "."}, {"-C", repo, "commit", "-qm", "two"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	r, err := Start(filepath.Join(repo, "pkg/web"), "fake", "sub job", now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(r.Dir, "pkg/web") {
		t.Errorf("dir: %s", r.Dir)
	}
	waitFor(t, "the tool", func() bool { _, err := os.Stat(filepath.Join(r.Dir, "where.txt")); return err == nil })
}

func TestStartRefusals(t *testing.T) {
	fakeTool(t, `sleep 30`)
	repo := gitRepo(t)
	for name, c := range map[string]struct{ project, tool, task, want string }{
		"empty task": {repo, "fake", "  ", "describe the task"},
		"flag task":  {repo, "fake", "--dangerously-skip-permissions go", "can't start with -"},
		"no tool":    {repo, "nope", "x", "unknown tool"},
		"not git":    {t.TempDir(), "fake", "x", "isn't a git repo"},
	} {
		_, err := Start(c.project, c.tool, c.task, now)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if b := branches(t, repo); b != "" {
		t.Errorf("a refused start left a branch: %s", b)
	}
}

func TestFailedLaunchCleansUp(t *testing.T) {
	fakeTool(t, `sleep 30`)
	repo := gitRepo(t)
	// A session already holding the name makes tmux refuse the launch.
	if err := setUp(); err != nil {
		t.Fatal(err)
	}
	if _, err := run(tmux("new-session", "-d", "-s", "doomed", "sleep", "30")); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(repo, "fake", "doomed", now); err == nil {
		t.Fatal("expected a failure")
	}
	if b := branches(t, repo); b != "" {
		t.Errorf("left a branch: %s", b)
	}
	if rs := loadRuns(); len(rs) != 0 {
		t.Errorf("left a run: %+v", rs)
	}
	out, _ := exec.Command("git", "-C", repo, "worktree", "list").CombinedOutput()
	if strings.Count(string(out), "\n") != 1 {
		t.Errorf("left a worktree:\n%s", out)
	}
}

func TestMergeAttachesTheRealSession(t *testing.T) {
	r := Run{Slug: "s", Project: "/p", Tool: "claude", Task: "the task", Worktree: "/wt/s", Dir: "/wt/s", Branch: "oryn/s"}
	found := []Session{
		{Tool: "claude", ID: "old", Dir: "/wt/s", Updated: now.Add(-time.Hour), State: Idle},
		{Tool: "claude", ID: "new", Dir: "/wt/s/sub", Updated: now, State: Working},
		{Tool: "claude", ID: "other", Dir: "/wt/s2", Updated: now},
	}
	ss := merge(found, []Run{r}, map[string]bool{"s": true}, nil, now)
	if len(ss) != 3 || ss[1].Run == nil || ss[1].Title != "the task" || ss[0].Run != nil {
		t.Fatalf("merged: %+v", ss)
	}
	if ss[0].Project != "/p" || ss[2].Project != "" {
		t.Errorf("projects: %q %q", ss[0].Project, ss[2].Project)
	}
	// A dead agent can't still be working, whatever its log last said.
	ss = merge([]Session{{Dir: "/wt/s", State: Working, Updated: now}}, []Run{r}, map[string]bool{"s": false}, nil, now)
	if ss[0].State != Idle || ss[0].Running {
		t.Errorf("dead: %+v", ss[0])
	}
}

func TestNoToolSkipsApprovals(t *testing.T) {
	for _, tool := range Tools {
		for _, a := range tool.Args("task") {
			if strings.Contains(a, "dangerously") || strings.Contains(a, "bypass") || strings.Contains(a, "skip-permissions") || strings.Contains(a, "yolo") {
				t.Errorf("%s launches with %s", tool.Name, a)
			}
		}
	}
}

func TestAgentStuckOnAPromptShowsWaiting(t *testing.T) {
	fakeTool(t, `printf 'Quick safety check\n> No, exit\n  Yes, I trust this folder\nEnter to confirm\n'; sleep 30`)
	r, err := Start(gitRepo(t), "fake", "needs a yes", now)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the prompt", func() bool { return askingNow(panes())[r.Slug] != "" })
	alive := panes()
	ss := merge(nil, loadRuns(), alive, askingNow(alive), now)
	if ss[0].State != Waiting || !strings.Contains(ss[0].Needs, "press a") {
		t.Errorf("stuck agent: %+v", ss[0])
	}
}

func TestAskedOnOnlyLooksAtTheBottom(t *testing.T) {
	if askedOn("Do you want to proceed?\n> Yes\n") == "" {
		t.Error("missed a prompt")
	}
	old := "Do you want to proceed?\n" + strings.Repeat("did some work\n", 20)
	if got := askedOn(old); got != "" {
		t.Errorf("an answered prompt further up still counted: %q", got)
	}
	if askedOn("all done\n") != "" {
		t.Error("false alarm")
	}
}

// bareRemote gives a repo an origin to push to, and a fake gh on PATH.
func bareRemote(t *testing.T, repo string) {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "origin.git")
	for _, args := range [][]string{{"init", "-q", "--bare", bare}, {"-C", repo, "remote", "add", "origin", bare}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	bin := t.TempDir()
	write(t, filepath.Join(bin, "gh"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$GH_ARGS\"\necho https://github.com/o/r/pull/7\n")
	os.Chmod(filepath.Join(bin, "gh"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_ARGS", filepath.Join(bin, "args"))
}

func commitIn(t *testing.T, dir, file, body string) {
	t.Helper()
	write(t, filepath.Join(dir, file), body)
	for _, args := range [][]string{{"-C", dir, "add", "."}, {"-C", dir, "-c", "user.email=a@a", "-c", "user.name=a", "commit", "-qm", "work"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
}

func TestReviewShipDiscard(t *testing.T) {
	fakeTool(t, `sleep 30`)
	repo := gitRepo(t)
	bareRemote(t, repo)
	r, err := Start(repo, "fake", "add the login page", now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Base == "" {
		t.Fatal("no base recorded")
	}

	if _, err := Ship(r); err == nil || !strings.Contains(err.Error(), "no commits") {
		t.Errorf("empty ship: %v", err)
	}

	write(t, filepath.Join(r.Worktree, "a.txt"), "changed")
	write(t, filepath.Join(r.Worktree, "new.txt"), "new")
	c, err := Review(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Diff, "+changed") || len(c.Untracked) != 1 || c.Untracked[0] != "new.txt" || c.Dirty != 2 || c.Commits != 0 {
		t.Errorf("review: %+v", c)
	}
	if _, err := Ship(r); err == nil || !strings.Contains(err.Error(), "2 uncommitted") {
		t.Errorf("dirty ship: %v", err)
	}

	commitIn(t, r.Worktree, "a.txt", "changed")
	url, err := Ship(r)
	if err != nil || url != "https://github.com/o/r/pull/7" {
		t.Fatalf("ship: %q %v", url, err)
	}
	args, _ := os.ReadFile(os.Getenv("GH_ARGS"))
	if !strings.Contains(string(args), "add the login page") || !strings.Contains(string(args), r.Branch) {
		t.Errorf("gh args: %s", args)
	}
	if strings.Contains(strings.ToLower(string(args)), "claude") {
		t.Errorf("attribution in PR: %s", args)
	}
	if rs := loadRuns(); rs[0].PR != url {
		t.Errorf("PR not saved: %+v", rs[0])
	}
	// Later work goes to the same PR.
	rs := loadRuns()
	commitIn(t, r.Worktree, "b.txt", "more")
	if again, err := Ship(rs[0]); err != nil || again != url {
		t.Errorf("second ship: %q %v", again, err)
	}

	if err := Discard(rs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.Worktree); err == nil {
		t.Error("worktree still there")
	}
	if branches(t, repo) != "" {
		t.Errorf("branch still there: %s", branches(t, repo))
	}
	if len(loadRuns()) != 0 {
		t.Error("run still recorded")
	}
	if panes()[r.Slug] {
		t.Error("agent still running")
	}
}

func TestDiscardOnlyTouchesOrynWorktrees(t *testing.T) {
	fakeTool(t, `sleep 30`)
	repo := gitRepo(t)
	err := Discard(Run{Slug: "x", Project: repo, Worktree: repo, Branch: "main"})
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("err: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "a.txt")); statErr != nil {
		t.Error("the project was touched")
	}
}

func TestReviewDoesNotRunAgentDiffDrivers(t *testing.T) {
	fakeTool(t, `sleep 30`)
	repo := gitRepo(t)
	r, err := Start(repo, "fake", "sneaky", now)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "RAN")
	exec.Command("git", "-C", r.Worktree, "config", "diff.external", "touch "+marker).Run()
	write(t, filepath.Join(r.Worktree, "a.txt"), "changed")
	if _, err := Review(r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("review ran the agent's diff driver")
	}
}
