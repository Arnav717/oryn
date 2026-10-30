package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Tool is an agent oryn can start. Args turns the task into its command line.
type Tool struct {
	Name string
	Bin  string
	Args func(task string) []string
}

// Tools are started in their normal interactive mode, so each keeps its own
// approval prompts. Never add a flag here that skips them.
var Tools = []Tool{
	{"claude", "claude", func(t string) []string { return []string{t} }},
	{"codex", "codex", func(t string) []string { return []string{t} }},
	{"opencode", "opencode", func(t string) []string { return []string{"--prompt", t} }},
	{"agy", "agy", func(t string) []string { return []string{"-i", t} }},
}

// InstalledTools names the tools found on PATH.
func InstalledTools() []string {
	var out []string
	for _, t := range Tools {
		if _, err := exec.LookPath(t.Bin); err == nil {
			out = append(out, t.Name)
		}
	}
	return out
}

// Run is an agent oryn started, in its own worktree and tmux session.
type Run struct {
	Slug     string    `json:"slug"`
	Project  string    `json:"project"`
	Tool     string    `json:"tool"`
	Task     string    `json:"task"`
	Worktree string    `json:"worktree"` // the worktree's root
	Dir      string    `json:"dir"`      // where the agent runs, under Worktree
	Branch   string    `json:"branch"`
	Base     string    `json:"base"` // the commit it started from
	PR       string    `json:"pr,omitempty"`
	Started  time.Time `json:"started"`
}

var (
	stateDir string
	// socket is a tmux server of oryn's own, so agents never mix with your
	// sessions and your tmux config can't change how they behave.
	socket = "oryn"
)

// SetStateDir sets where runs and worktrees are kept.
func SetStateDir(dir string) { stateDir = dir }

func runsPath() string { return filepath.Join(stateDir, "agents.json") }

func loadRuns() []Run {
	var runs []Run
	if stateDir != "" {
		readJSON(runsPath(), &runs)
	}
	return runs
}

func saveRuns(runs []Run) error {
	b, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		return err
	}
	tmp := runsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, runsPath())
}

func tmux(args ...string) *exec.Cmd {
	return exec.Command("tmux", append([]string{"-f", os.DevNull, "-L", socket}, args...)...)
}

func run(cmd *exec.Cmd) (string, error) {
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if err != nil && text != "" {
		return text, errors.New(text)
	}
	return text, err
}

// setUp configures the server: ctrl+] leaves an agent without a prefix key,
// which also works when oryn itself runs inside tmux, and a finished agent's
// last screen stays readable.
func setUp() error {
	_, err := run(tmux("start-server",
		";", "set-option", "-g", "exit-empty", "off",
		";", "set-option", "-g", "remain-on-exit", "on",
		";", "set-option", "-g", "escape-time", "0",
		";", "set-option", "-g", "mouse", "on",
		";", "set-option", "-g", "default-terminal", "tmux-256color",
		";", "set-option", "-ga", "terminal-features", ",*:RGB",
		";", "set-option", "-g", "status-style", "bg=#10141c,fg=#7d8796",
		";", "set-option", "-g", "status-left", "",
		";", "set-option", "-g", "status-right", " ctrl+] back to oryn ",
		";", "set-option", "-g", "window-status-format", "",
		";", "set-option", "-g", "window-status-current-format", "",
		";", "bind-key", "-n", "C-]", "detach-client"))
	return err
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugFor makes a short branch-safe name from the task's first few words.
func slugFor(task string) string {
	words := strings.Fields(notSlug.ReplaceAllString(strings.ToLower(task), " "))
	s := strings.Join(words[:min(4, len(words))], "-")
	if s == "" {
		return "task"
	}
	return s
}

// uniqueSlug avoids names in use by a run or an existing branch.
func uniqueSlug(top, base string, runs []Run) string {
	taken := func(s string) bool {
		if slices.ContainsFunc(runs, func(r Run) bool { return r.Slug == s }) {
			return true
		}
		return exec.Command("git", "-C", top, "show-ref", "--verify", "--quiet", "refs/heads/oryn/"+s).Run() == nil
	}
	s := base
	for i := 2; taken(s); i++ {
		s = fmt.Sprintf("%s-%d", base, i)
	}
	return s
}

// Dirty reports whether a project has changes a new worktree won't include.
func Dirty(project string) bool {
	out, err := run(exec.Command("git", "-C", project, "status", "--porcelain"))
	return err == nil && out != ""
}

// Start creates a worktree and branch from the project's HEAD and launches the
// tool in it. The task reaches the tool as its own argument: tmux runs a
// command given as several arguments directly, with no shell to interpret it.
func Start(project, toolName, task string, now time.Time) (Run, error) {
	task = strings.TrimSpace(task)
	i := slices.IndexFunc(Tools, func(t Tool) bool { return t.Name == toolName })
	switch {
	case task == "":
		return Run{}, errors.New("describe the task first")
	case strings.HasPrefix(task, "-"):
		return Run{}, errors.New("a task can't start with - (the tool would read it as a flag)")
	case i < 0:
		return Run{}, fmt.Errorf("unknown tool %s", toolName)
	case stateDir == "":
		return Run{}, errors.New("no state directory set")
	}
	tool := Tools[i]
	if _, err := exec.LookPath("tmux"); err != nil {
		return Run{}, errors.New("tmux not found: oryn runs agents inside it, so install tmux first")
	}
	if _, err := exec.LookPath(tool.Bin); err != nil {
		return Run{}, fmt.Errorf("%s not found on PATH", tool.Bin)
	}

	top, err := run(exec.Command("git", "-C", project, "rev-parse", "--show-toplevel"))
	if err != nil {
		return Run{}, fmt.Errorf("%s isn't a git repo, and agents need one for their own branch", filepath.Base(project))
	}
	prefix, _ := run(exec.Command("git", "-C", project, "rev-parse", "--show-prefix"))
	base, err := git(top, "rev-parse", "HEAD")
	if err != nil {
		return Run{}, fmt.Errorf("%s has no commits yet", filepath.Base(project))
	}

	runs := loadRuns()
	slug := uniqueSlug(top, slugFor(task), runs)
	r := Run{
		Slug: slug, Project: project, Tool: tool.Name, Task: task, Started: now,
		Worktree: filepath.Join(stateDir, "worktrees", filepath.Base(project), slug),
		Branch:   "oryn/" + slug, Base: base,
	}
	r.Dir = filepath.Join(r.Worktree, prefix)
	if err := os.MkdirAll(filepath.Dir(r.Worktree), 0o755); err != nil {
		return Run{}, err
	}
	if _, err := run(exec.Command("git", "-C", top, "worktree", "add", "-b", r.Branch, r.Worktree)); err != nil {
		return Run{}, fmt.Errorf("creating worktree: %w", err)
	}
	undo := func(cause error) (Run, error) {
		run(exec.Command("git", "-C", top, "worktree", "remove", "--force", r.Worktree))
		run(exec.Command("git", "-C", top, "branch", "-D", r.Branch))
		return Run{}, cause
	}
	if err := setUp(); err != nil {
		return undo(fmt.Errorf("starting tmux: %w", err))
	}
	args := append([]string{"new-session", "-d", "-s", slug, "-c", r.Dir, "-x", "160", "-y", "50", tool.Bin}, tool.Args(task)...)
	if _, err := run(tmux(args...)); err != nil {
		return undo(fmt.Errorf("starting %s: %w", tool.Name, err))
	}
	if err := saveRuns(append(runs, r)); err != nil {
		run(tmux("kill-session", "-t", "="+slug))
		return undo(err)
	}
	return r, nil
}

// ProjectOf maps one of oryn's worktrees back to its project, so an agent
// running in one gets its project's context. Any other folder returns "".
func ProjectOf(dir string) string {
	for _, r := range loadRuns() {
		if dir == r.Worktree || strings.HasPrefix(dir, r.Worktree+string(filepath.Separator)) {
			return r.Project
		}
	}
	return ""
}

// Stop ends the agent. Its worktree and branch stay, so nothing is lost.
func Stop(r Run) error {
	_, err := run(tmux("kill-session", "-t", "="+r.Slug))
	return err
}

// AttachCmd opens the agent's live terminal. TMUX is cleared so this also
// works from inside your own tmux.
func AttachCmd(r Run) *exec.Cmd {
	cmd := tmux("attach-session", "-t", "="+r.Slug)
	cmd.Env = slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(e, "TMUX=") })
	return cmd
}

// panes maps each tmux session on oryn's server to whether its program is
// still running.
func panes() map[string]bool {
	out, err := run(tmux("list-panes", "-a", "-F", "#{session_name} #{pane_dead}"))
	if err != nil {
		return nil
	}
	alive := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if name, dead, ok := strings.Cut(l, " "); ok {
			alive[name] = dead == "0"
		}
	}
	return alive
}

// asking is what agents put on screen when they're stuck on you. oryn owns
// these terminals, so it can see this even for tools that don't record it.
var asking = []struct{ text, needs string }{
	{"trust this folder", "Approve this folder: press a and confirm"},
	{"Do you trust", "Approve this folder: press a and confirm"},
	{"Do you want to", "Approve a step: press a to answer"},
	{"Allow command?", "Approve a command: press a to answer"},
	{"Enter to confirm", "Waiting for a choice: press a to answer"},
}

// askingNow reads the bottom of each running agent's screen.
func askingNow(alive map[string]bool) map[string]string {
	asks := map[string]string{}
	for slug, running := range alive {
		if !running {
			continue
		}
		if out, err := run(tmux("capture-pane", "-p", "-t", slug)); err == nil {
			if need := askedOn(out); need != "" {
				asks[slug] = need
			}
		}
	}
	return asks
}

func askedOn(screen string) string {
	var tail []string
	lines := strings.Split(screen, "\n")
	for i := len(lines) - 1; i >= 0 && len(tail) < 12; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			tail = append(tail, l)
		}
	}
	text := strings.Join(tail, "\n")
	for _, a := range asking {
		if strings.Contains(text, a.text) {
			return a.needs
		}
	}
	return ""
}

// merge ties runs to what the adapters found in their worktrees. A run the
// adapters can't see yet still shows, from what oryn knows itself.
func merge(sessions []Session, runs []Run, alive map[string]bool, asks map[string]string, now time.Time) []Session {
	for _, r := range runs {
		best := -1
		for i := range sessions {
			s := &sessions[i]
			if s.Dir != r.Worktree && !strings.HasPrefix(s.Dir, r.Worktree+string(filepath.Separator)) {
				continue
			}
			s.Project = r.Project
			if best < 0 || s.Updated.After(sessions[best].Updated) {
				best = i
			}
		}
		running := alive[r.Slug]
		if best < 0 {
			st := Idle
			if running {
				st = Working
			}
			sessions = append(sessions, Session{Tool: r.Tool, ID: "oryn-" + r.Slug, Dir: r.Dir, Updated: r.Started, State: st})
			best = len(sessions) - 1
			sessions[best].Project = r.Project
		}
		s := &sessions[best]
		rc := r
		s.Run, s.Running, s.Title = &rc, running, r.Task
		if !running && s.State == Working {
			s.State = Idle
		}
		if need := asks[r.Slug]; need != "" {
			s.State, s.Needs = Waiting, need
		}
	}
	return sessions
}
