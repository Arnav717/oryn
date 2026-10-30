package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Arnav717/oryn/internal/agents"
	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/brief"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/mcp"
	"github.com/Arnav717/oryn/internal/store"
)

func hereOr(args []string) string {
	if len(args) > 0 {
		return config.ExpandHome(args[0])
	}
	wd, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	return wd
}

func briefFor(cfg *config.Config, st *store.Store, dir string) brief.Project {
	now := time.Now()
	return brief.For(brief.Resolve(cfg, dir), app.LoadCache(), agents.List(now), st, dir, now)
}

func contextCmd(cfg *config.Config, st *store.Store, args []string, asJSON bool) {
	b := briefFor(cfg, st, hereOr(args))
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(b); err != nil {
			fatal(err)
		}
		return
	}
	fmt.Printf("%s  %s\n", b.Name, config.ShortHome(b.Path))
	if len(b.Stack) > 0 {
		fmt.Println("stack    ", strings.Join(b.Stack, ", "))
	}
	keys := make([]string, 0, len(b.Commands))
	for k := range b.Commands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%-9s %s\n", k, b.Commands[k])
	}
	fmt.Printf("git       %s, %d uncommitted\n", orDash(b.Git.Branch), b.Git.Uncommitted)
	if g := b.GitHub; g != nil {
		fmt.Printf("github    %s, ci %s, %d PRs, %d issues\n", g.Repo, orDash(g.CI), g.OpenPRs, g.OpenIssues)
	}
	for _, d := range b.Deploys {
		fmt.Printf("deploy    %s %s %s\n", d.Platform, orDash(d.State), d.URL)
	}
	for _, a := range b.Agents {
		fmt.Printf("agent     %s %s: %s\n", a.Tool, a.State, a.Task)
	}
	for _, c := range b.Claims {
		fmt.Printf("claimed   %s by %s\n", c.Path, c.Agent)
	}
	if b.DataAt.IsZero() {
		fmt.Println("\nno remote data yet: open oryn once to fill it in")
	}
}

func claimCmd(cfg *config.Config, st *store.Store, agent string, paths []string, release bool) {
	if agent == "" {
		fatal(errors.New("say who is claiming: --as NAME"))
	}
	if len(paths) == 0 && !release {
		fatal(errors.New("nothing to claim: give one or more paths"))
	}
	project := brief.Resolve(cfg, hereOr(nil))
	if release {
		if err := st.Release(project, agent, paths); err != nil {
			fatal(err)
		}
		fmt.Println("released")
		return
	}
	conflicts, err := st.Claim(project, agent, paths, time.Now())
	if err != nil {
		fatal(err)
	}
	if len(conflicts) > 0 {
		for _, c := range conflicts {
			fmt.Fprintf(os.Stderr, "%s is claimed by %s\n", c.Path, c.Agent)
		}
		os.Exit(1)
	}
	fmt.Println("claimed for two hours:", strings.Join(paths, " "))
}

func object(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var (
	pathProp  = map[string]any{"type": "string", "description": "Folder inside the project. Defaults to where the server runs."}
	agentProp = map[string]any{"type": "string", "description": "A stable name for you, such as claude-login-fix."}
	filesProp = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Paths relative to the project root. A folder covers everything inside it."}
)

type claimInput struct {
	Path  string   `json:"path"`
	Agent string   `json:"agent"`
	Files []string `json:"files"`
}

func mcpTools(cfg *config.Config, st *store.Store) []mcp.Tool {
	project := func(a claimInput) string {
		if a.Path == "" {
			a.Path = hereOr(nil)
		}
		return brief.Resolve(cfg, config.ExpandHome(a.Path))
	}
	input := func(raw json.RawMessage) (claimInput, error) {
		var a claimInput
		err := json.Unmarshal(raw, &a)
		return a, err
	}
	return []mcp.Tool{
		{
			Name:        "project_context",
			Description: "Everything needed to start working in a project: stack, commands to run, top-level layout, git state, CI, deploys, other active agents and claimed files. Call this first instead of exploring.",
			Schema:      object(map[string]any{"path": pathProp}),
			Run: func(raw json.RawMessage) (any, error) {
				a, err := input(raw)
				if err != nil {
					return nil, err
				}
				dir := a.Path
				if dir == "" {
					dir = hereOr(nil)
				}
				return briefFor(cfg, st, config.ExpandHome(dir)), nil
			},
		},
		{
			Name:        "list_projects",
			Description: "The projects watched in oryn.",
			Schema:      object(map[string]any{}),
			Run: func(json.RawMessage) (any, error) {
				return slices.Clone(cfg.Projects), nil
			},
		},
		{
			Name:        "list_agents",
			Description: "AI agents waiting or running in watched projects, with their tasks and branches. Use it to avoid duplicating work.",
			Schema:      object(map[string]any{}),
			Run: func(json.RawMessage) (any, error) {
				type row struct {
					Project string `json:"project"`
					brief.Agent
				}
				out := []row{}
				for _, s := range agents.List(time.Now()) {
					p := brief.Resolve(cfg, s.Dir)
					if !s.Active() || !slices.Contains(cfg.Projects, p) {
						continue
					}
					a := brief.Agent{Tool: s.Tool, State: s.State.String(), Task: s.Title, Updated: s.Updated}
					if s.Run != nil {
						a.Branch = s.Run.Branch
					}
					out = append(out, row{p, a})
				}
				return out, nil
			},
		},
		{
			Name:        "claim_files",
			Description: "Tell other agents you're working on these paths. All or nothing: if another agent holds one, nothing is claimed and the conflicts come back. Claims last two hours.",
			Schema:      object(map[string]any{"path": pathProp, "agent": agentProp, "files": filesProp}, "agent", "files"),
			Run: func(raw json.RawMessage) (any, error) {
				a, err := input(raw)
				if err != nil {
					return nil, err
				}
				if a.Agent == "" || len(a.Files) == 0 {
					return nil, errors.New("agent and files are required")
				}
				conflicts, err := st.Claim(project(a), a.Agent, a.Files, time.Now())
				if err != nil {
					return nil, err
				}
				return map[string]any{"claimed": len(conflicts) == 0, "conflicts": conflicts}, nil
			},
		},
		{
			Name:        "release_files",
			Description: "Give up your claims when you're done. With no files, releases all of yours.",
			Schema:      object(map[string]any{"path": pathProp, "agent": agentProp, "files": filesProp}, "agent"),
			Run: func(raw json.RawMessage) (any, error) {
				a, err := input(raw)
				if err != nil {
					return nil, err
				}
				if a.Agent == "" {
					return nil, errors.New("agent is required")
				}
				return map[string]any{"released": true}, st.Release(project(a), a.Agent, a.Files)
			},
		},
	}
}
