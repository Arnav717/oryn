package agents

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Changes is what an agent did in its worktree, measured from where it started.
type Changes struct {
	Diff      string
	Untracked []string // new files git isn't tracking yet
	Commits   int
	Dirty     int // files changed but not committed
}

const maxDiff = 2 << 20

func git(dir string, args ...string) (string, error) {
	return run(exec.Command("git", append([]string{"-C", dir}, args...)...))
}

func baseOf(r Run) string {
	if r.Base != "" {
		return r.Base
	}
	base, _ := git(r.Project, "rev-parse", "HEAD")
	return base
}

// Review reads a run's work. External diff and textconv drivers are off: the
// agent could have configured them, and reviewing shouldn't run its code.
func Review(r Run) (Changes, error) {
	base := baseOf(r)
	diff, err := git(r.Worktree, "diff", "--no-color", "--no-ext-diff", "--no-textconv", base)
	if err != nil {
		return Changes{}, err
	}
	if len(diff) > maxDiff {
		diff = diff[:maxDiff] + "\n… diff cut off at 2 MB"
	}
	c := Changes{Diff: diff}
	if out, err := git(r.Worktree, "ls-files", "--others", "--exclude-standard"); err == nil && out != "" {
		c.Untracked = strings.Split(out, "\n")
	}
	if out, err := git(r.Worktree, "rev-list", "--count", base+"..HEAD"); err == nil {
		c.Commits, _ = strconv.Atoi(out)
	}
	if out, err := git(r.Worktree, "status", "--porcelain"); err == nil && out != "" {
		c.Dirty = len(strings.Split(out, "\n"))
	}
	return c, nil
}

// Ship pushes the run's branch and opens a PR, or updates the one it has.
// Uncommitted work would silently be left out, so that refuses.
func Ship(r Run) (string, error) {
	c, err := Review(r)
	switch {
	case err != nil:
		return "", err
	case c.Dirty > 0:
		return "", fmt.Errorf("%d uncommitted file(s) in the worktree: press a and ask the agent to commit", c.Dirty)
	case c.Commits == 0:
		return "", errors.New("no commits to push yet")
	}
	if _, err := exec.LookPath("gh"); err != nil && r.PR == "" {
		return "", errors.New("gh not found: oryn opens PRs with the GitHub CLI")
	}
	if _, err := git(r.Worktree, "push", "-u", "origin", r.Branch); err != nil {
		return "", fmt.Errorf("push failed: %w", err)
	}
	if r.PR != "" {
		return r.PR, nil
	}
	title := oneLine(r.Task)
	if len(title) > 72 {
		title = title[:72]
	}
	cmd := exec.Command("gh", "pr", "create", "--head", r.Branch, "--title", title, "--body", r.Task)
	cmd.Dir = r.Worktree
	out, err := run(cmd)
	if err != nil {
		return "", fmt.Errorf("pushed, but the PR failed: %w", err)
	}
	lines := strings.Split(out, "\n")
	url := lines[len(lines)-1]
	return url, updateRun(r.Slug, func(x *Run) { x.PR = url })
}

// Discard stops the agent and deletes its worktree and local branch. A branch
// already pushed stays on the remote.
func Discard(r Run) error {
	root := filepath.Join(stateDir, "worktrees") + string(filepath.Separator)
	if stateDir == "" || !strings.HasPrefix(r.Worktree, root) {
		return fmt.Errorf("refusing to delete %s: it isn't one of oryn's worktrees", r.Worktree)
	}
	Stop(r) // already stopped is fine
	if _, err := os.Stat(r.Worktree); err == nil {
		if _, err := git(r.Project, "worktree", "remove", "--force", r.Worktree); err != nil {
			return err
		}
	}
	git(r.Project, "worktree", "prune")
	if _, err := git(r.Project, "branch", "-D", r.Branch); err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return saveRuns(slices.DeleteFunc(loadRuns(), func(x Run) bool { return x.Slug == r.Slug }))
}

func updateRun(slug string, fn func(*Run)) error {
	runs := loadRuns()
	for i := range runs {
		if runs[i].Slug == slug {
			fn(&runs[i])
		}
	}
	return saveRuns(runs)
}
