// Command oryn is a terminal hub for your projects.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/agents"
	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/deploy"
	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
	"github.com/Arnav717/oryn/internal/tui"
)

// version is overridden at release time with -ldflags "-X main.version=...".
var version = "0.5.1"

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

const usage = `oryn - a terminal hub for your projects

Usage:
  oryn              open the terminal (home screen)
  oryn add [dir]    watch a project (default: current dir)
  oryn rm [dir]     stop watching a project
  oryn ls           list watched projects
  oryn scan         print a report and exit
  oryn deploys      show where each project deploys (Vercel, Netlify, Cloudflare Pages)
  oryn daemon       refresh in the background, send desktop alerts
  oryn version      print the version

Flags:
`

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "version" || args[0] == "-v" || args[0] == "--version") {
		fmt.Println("oryn", version)
		return
	}
	cmd := "tui"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet("oryn", flag.ExitOnError)
	var projects multiFlag
	fs.Var(&projects, "p", "project directory to watch (repeatable; default: config or current dir)")
	cfgPath := fs.String("config", config.DefaultPath(), "config file")
	dbPath := fs.String("db", store.DefaultPath(), "event database")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage); fs.PrintDefaults() }
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatal(err)
	}
	deploy.SetCredsDir(filepath.Dir(*cfgPath))
	agents.SetStateDir(filepath.Dir(*dbPath))

	switch cmd {
	case "add", "a":
		editProjects(cfg, fs.Args(), true)
		return
	case "rm", "remove":
		editProjects(cfg, fs.Args(), false)
		return
	case "ls", "list":
		listProjects(cfg)
		return
	case "daemon":
		daemon(*cfgPath, *dbPath)
		return
	}

	if len(projects) > 0 {
		cfg.Projects = nil
		cfg.Ephemeral = true // -p is a one-off view; the TUI must not persist it
		for _, p := range projects {
			cfg.Projects = append(cfg.Projects, config.ExpandHome(p))
		}
	}
	// The TUI's home screen handles "nothing watched"; scan needs something to read.
	if len(cfg.Projects) == 0 && cmd != "tui" {
		wd, err := os.Getwd()
		if err != nil {
			fatal(err)
		}
		cfg.Projects = []string{wd}
	}
	for i, p := range cfg.Projects {
		if abs, err := filepath.Abs(p); err == nil {
			cfg.Projects[i] = abs
		}
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fatal(err)
	}
	defer st.Close()

	switch cmd {
	case "tui":
		p := tea.NewProgram(tui.New(cfg, st), tea.WithAltScreen())
		_, err := p.Run()
		// Mark the end of this visit so only events from while you were away
		// count as new next time.
		st.SetMeta("last_seen", strconv.FormatInt(time.Now().Unix(), 10))
		if err != nil {
			fatal(err)
		}
	case "scan", "s":
		scan(cfg, st)
	case "deploys", "deploy":
		deploys(cfg)
	default:
		fs.Usage()
		os.Exit(2)
	}
}

func scan(cfg *config.Config, st *store.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	snap := app.Refresh(ctx, cfg, st)

	direct := 0
	for _, d := range snap.Deps {
		if d.Direct {
			direct++
		}
	}
	fmt.Printf("%d deps (%d direct) across %d project(s)\n\n", len(snap.Deps), direct, len(cfg.Projects))

	vulns := snap.VulnList()
	fmt.Printf("VULNERABILITIES (%d)\n", len(vulns))
	for _, v := range vulns {
		fix := ""
		if v.Fixed != "" {
			fix = " -> " + v.Fixed
		}
		fmt.Printf("  %-8s %-20s %s@%s%s  %s\n", v.Severity, v.CVE(), v.Package, v.Version, fix, v.Summary)
	}

	fmt.Println("\nOUTDATED (direct)")
	seen := map[string]bool{}
	for _, d := range snap.Deps {
		k := source.PkgKey(d.Ecosystem, d.Name)
		if d.Direct && !seen[k] && source.Outdated(d.Version, snap.Latest[k]) {
			seen[k] = true
			fmt.Printf("  %-40s %s -> %s\n", d.Name, d.Version, snap.Latest[k])
		}
	}

	fmt.Println("\nUPSTREAM")
	for _, s := range snap.Status {
		desc := s.Description
		if s.Err != nil {
			desc = "error: " + s.Err.Error()
		}
		fmt.Printf("  %-14s %s\n", s.Service, desc)
	}
	fmt.Println("\nGITHUB")
	switch gh := snap.GitHub; {
	case errors.Is(snap.GitHubErr, source.ErrNoToken):
		fmt.Println("  not connected (run `gh auth login`)")
	case snap.GitHubErr != nil:
		fmt.Println("  error:", snap.GitHubErr)
	case gh == nil:
		fmt.Println("  disabled in config")
	default:
		fmt.Printf("  %s: %d awaiting your review, %d open PRs of yours\n", gh.Login, len(gh.Review), len(gh.Mine))
		for _, pr := range gh.Review {
			fmt.Printf("  review  %-34s %s\n", pr.Ref(), pr.Title)
		}
		for _, pr := range gh.Mine {
			fmt.Printf("  mine    %-34s ci=%-8s %s\n", pr.Ref(), orDash(pr.CI), pr.Title)
		}
	}
	if len(snap.RepoRefs) > 0 {
		fmt.Println("\nREPOS")
		for _, p := range cfg.Projects {
			ref, ok := snap.RepoRefs[p]
			if !ok {
				continue
			}
			if d := snap.Repos[p]; d != nil {
				fmt.Printf("  %-28s %d PRs, %d issues, %d runs, %d people, %d commits, ci=%s, alerts=%s\n",
					ref, d.OpenPRs, d.OpenIssues, len(d.Runs), len(d.People), len(d.Commits), orDash(d.DefaultCI), alertsText(d))
			} else if err := snap.RepoErrs[p]; err != nil {
				fmt.Printf("  %-28s error: %v\n", ref, err)
			}
		}
	}
	for _, w := range snap.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
}

func alertsText(d *source.RepoData) string {
	if !d.AlertsKnown {
		return "n/a"
	}
	return fmt.Sprint(d.AlertCount)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// editProjects adds or removes dirs (default: current dir) and saves the config.
func editProjects(cfg *config.Config, dirs []string, add bool) {
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	for _, d := range dirs {
		abs, err := filepath.Abs(config.ExpandHome(d))
		if err != nil {
			fatal(err)
		}
		i := slices.Index(cfg.Projects, abs)
		switch {
		case add && i >= 0:
			fmt.Println("already watching", config.ShortHome(abs))
		case add:
			if st, err := os.Stat(abs); err != nil || !st.IsDir() {
				fatal(fmt.Errorf("%s is not a directory", d))
			}
			cfg.Projects = append(cfg.Projects, abs)
			fmt.Println("watching", config.ShortHome(abs))
		case i >= 0:
			cfg.Projects = slices.Delete(cfg.Projects, i, i+1)
			fmt.Println("stopped watching", config.ShortHome(abs))
		default:
			fmt.Println("not watching", config.ShortHome(abs))
		}
	}
	if err := cfg.Save(); err != nil {
		fatal(err)
	}
}

func listProjects(cfg *config.Config) {
	if len(cfg.Projects) == 0 {
		fmt.Println("no watched projects; `oryn add` in a project dir to start")
		return
	}
	for _, p := range cfg.Projects {
		fmt.Println(config.ShortHome(p))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "oryn:", err)
	os.Exit(1)
}
