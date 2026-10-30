// Package brief builds a compact picture of a project for AI agents, so they
// can start working without exploring. It reads local files and oryn's last
// refresh, never the network, and carries no text written by outsiders such as
// PR titles or logs: counts, states, and names from files committed to the
// project itself, the same as its script names.
package brief

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Arnav717/oryn/internal/agents"
	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/store"
)

type Deploy struct {
	Platform string    `json:"platform"`
	URL      string    `json:"url,omitempty"`
	State    string    `json:"state,omitempty"`
	At       time.Time `json:"deployed_at,omitzero"`
}

type Agent struct {
	Tool    string    `json:"tool"`
	State   string    `json:"state"`
	Task    string    `json:"task,omitempty"`
	Branch  string    `json:"branch,omitempty"`
	Updated time.Time `json:"updated"`
	// SameFolder marks one running exactly where the caller is, which may be
	// the caller itself: sessions don't identify themselves.
	SameFolder bool `json:"same_folder,omitempty"`
}

type GitHub struct {
	Repo          string     `json:"repo"`
	DefaultBranch string     `json:"default_branch,omitempty"`
	CI            string     `json:"ci,omitempty"`
	OpenPRs       int        `json:"open_prs"`
	OpenIssues    int        `json:"open_issues"`
	Failure       *CIFailure `json:"ci_failure,omitempty"`
}

// CIFailure says where CI broke on the default branch head and how to read the
// log. The log itself stays out: an agent should go to it knowingly, as data.
type CIFailure struct {
	Workflow   string `json:"workflow,omitempty"`
	Job        string `json:"job,omitempty"`
	Step       string `json:"step,omitempty"`
	RunID      int64  `json:"run_id"`
	LogCommand string `json:"log_command"`
}

type Git struct {
	Branch      string `json:"branch,omitempty"`
	Uncommitted int    `json:"uncommitted"`
}

// Project is what an agent gets back.
type Project struct {
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	Stack    []string          `json:"stack,omitempty"`
	Commands map[string]string `json:"commands,omitempty"`
	Scripts  []string          `json:"other_scripts,omitempty"`
	Guides   []string          `json:"guides,omitempty"`
	Layout   []string          `json:"layout,omitempty"`
	Git      Git               `json:"git"`
	GitHub   *GitHub           `json:"github,omitempty"`
	Deploys  []Deploy          `json:"deploys,omitempty"`
	Agents   []Agent           `json:"other_agents,omitempty"`
	Claims   []store.Claim     `json:"claims,omitempty"`
	DataAt   time.Time         `json:"remote_data_at,omitzero"`
}

// Resolve maps a folder to its project: an agent's worktree to the project it
// came from, a folder inside a watched project to that project.
func Resolve(cfg *config.Config, dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if p := agents.ProjectOf(abs); p != "" {
		return p
	}
	best := ""
	for _, p := range cfg.Projects {
		if (abs == p || strings.HasPrefix(abs, p+string(filepath.Separator))) && len(p) > len(best) {
			best = p
		}
	}
	if best != "" {
		return best
	}
	return abs
}

// For builds a project's brief. snap and st may be nil. self is the folder
// asking, so an agent isn't told about itself.
func For(project string, snap *app.Snapshot, sessions []agents.Session, st *store.Store, self string, now time.Time) Project {
	b := Project{Name: filepath.Base(project), Path: project}
	b.Stack, b.Commands = detectStack(project)
	b.Scripts = otherScripts(project, b.Commands)
	b.Guides = guides(project)
	b.Layout = layout(project)
	b.Git = gitState(self)

	if snap != nil {
		b.DataAt = snap.At
		if ref, ok := snap.RepoRefs[project]; ok {
			b.GitHub = &GitHub{Repo: ref.String()}
			if d := snap.Repos[project]; d != nil {
				b.GitHub.DefaultBranch, b.GitHub.CI = d.DefaultBranch, d.DefaultCI
				b.GitHub.OpenPRs, b.GitHub.OpenIssues = d.OpenPRs, d.OpenIssues
				if f := d.Failure; f != nil {
					b.GitHub.Failure = &CIFailure{
						Workflow: safeName(f.Workflow), Job: safeName(f.Job), Step: safeName(f.Step), RunID: f.RunID,
						LogCommand: fmt.Sprintf("gh run view %d --repo %s --log-failed", f.RunID, ref),
					}
				}
			}
		}
		for _, t := range snap.Deploys {
			if t.Link.Project != project {
				continue
			}
			d := Deploy{Platform: t.Link.Platform, URL: t.LiveURL}
			if x, ok := t.Latest("production"); ok {
				d.State, d.At = x.State, x.Created
				if d.URL == "" {
					d.URL = x.URL
				}
			}
			b.Deploys = append(b.Deploys, d)
		}
	}

	for _, s := range sessions {
		if !s.Active() {
			continue
		}
		if s.Project != project && s.Dir != project && !strings.HasPrefix(s.Dir, project+string(filepath.Separator)) {
			continue
		}
		a := Agent{Tool: s.Tool, State: s.State.String(), Task: s.Title, Updated: s.Updated, SameFolder: s.Dir == self}
		if s.Run != nil {
			a.Branch = s.Run.Branch
		}
		b.Agents = append(b.Agents, a)
	}
	if st != nil {
		b.Claims, _ = st.Claims(project, now)
	}
	return b
}

// safeName keeps a workflow name to what names are made of, so one built from
// an expression can't carry a line break or a paragraph into an agent's context.
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" ._:/()+-", r) {
			return r
		}
		return -1
	}, s)
	if r := []rune(s); len(r) > 80 {
		s = string(r[:80])
	}
	return strings.TrimSpace(s)
}

func gitOut(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

func gitState(dir string) Git {
	g := Git{Branch: gitOut(dir, "branch", "--show-current")}
	if out := gitOut(dir, "status", "--porcelain"); out != "" {
		g.Uncommitted = len(strings.Split(out, "\n"))
	}
	return g
}

// layout is the project's top level as git knows it, so ignored junk stays out.
func layout(dir string) []string {
	out := gitOut(dir, "ls-tree", "--name-only", "HEAD")
	if out == "" {
		return nil
	}
	names := strings.Split(out, "\n")
	return names[:min(60, len(names))]
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func detectStack(dir string) ([]string, map[string]string) {
	var stack []string
	cmds := map[string]string{}
	set := func(k, v string) {
		if _, ok := cmds[k]; !ok {
			cmds[k] = v
		}
	}
	if exists(dir, "Makefile") {
		for _, t := range makeTargets(dir) {
			switch t {
			case "test", "build", "lint", "run", "dev", "install":
				set(t, "make "+t)
			}
		}
	}
	if exists(dir, "go.mod") {
		stack = append(stack, "go")
		set("test", "go test ./...")
		set("build", "go build ./...")
	}
	if exists(dir, "Cargo.toml") {
		stack = append(stack, "rust")
		set("test", "cargo test")
		set("build", "cargo build")
		set("run", "cargo run")
	}
	if exists(dir, "pyproject.toml") || exists(dir, "requirements.txt") {
		stack = append(stack, "python")
		run := ""
		if exists(dir, "uv.lock") {
			run = "uv run "
		} else if exists(dir, "poetry.lock") {
			run = "poetry run "
		}
		set("test", run+"pytest")
	}
	if exists(dir, "package.json") {
		stack = append(stack, "node")
		pm := "npm"
		switch {
		case exists(dir, "pnpm-lock.yaml"):
			pm = "pnpm"
		case exists(dir, "yarn.lock"):
			pm = "yarn"
		case exists(dir, "bun.lockb"), exists(dir, "bun.lock"):
			pm = "bun"
		}
		stack = append(stack, pm)
		set("install", pm+" install")
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil && json.Unmarshal(b, &pkg) == nil {
			for _, s := range []string{"dev", "build", "test", "lint", "start", "typecheck"} {
				if _, ok := pkg.Scripts[s]; ok {
					set(s, pm+" run "+s)
				}
			}
		}
	}
	slices.Sort(stack)
	return stack, cmds
}

// otherScripts names package.json scripts beyond the usual ones. Projects hide
// required steps here, such as a code generator to run before building.
func otherScripts(dir string, known map[string]string) []string {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil || json.Unmarshal(b, &pkg) != nil {
		return nil
	}
	var out []string
	for name := range pkg.Scripts {
		if _, ok := known[name]; !ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out[:min(30, len(out))]
}

// guides are the files where a project explains itself to agents and people.
func guides(dir string) []string {
	var out []string
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", "CONTRIBUTING.md", "README.md"} {
		if exists(dir, name) {
			out = append(out, name)
		}
	}
	return out
}

var makeTarget = regexp.MustCompile(`(?m)^([a-zA-Z][a-zA-Z0-9_-]*):`)

func makeTargets(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "Makefile"))
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range makeTarget.FindAllSubmatch(b, -1) {
		out = append(out, string(m[1]))
	}
	return out
}
