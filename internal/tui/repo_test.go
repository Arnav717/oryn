package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/gitrepo"
	"github.com/Arnav717/oryn/internal/source"
)

func repoModel(d *source.RepoData) Model {
	m := testModel("", "/code/a")
	m.w, m.h = 140, 30
	m.scope, m.screen, m.tab = "/code/a", screenProject, tabGitHub
	m.snap = &app.Snapshot{
		RepoRefs: map[string]gitrepo.Ref{"/code/a": {Owner: "o", Name: "r"}},
		Repos:    map[string]*source.RepoData{"/code/a": d},
		RepoErrs: map[string]error{},
		GitHub:   &source.GitHubData{Login: "me"},
	}
	return m
}

func fakeRepo(prs, issues int) *source.RepoData {
	d := &source.RepoData{Ref: gitrepo.Ref{Owner: "o", Name: "r"}, Viewer: "me", URL: "https://github.com/o/r", DefaultBranch: "main", AlertsKnown: true}
	for i := 1; i <= prs; i++ {
		d.PRs = append(d.PRs, source.PR{Repo: "o/r", Number: i, Title: fmt.Sprintf("pr %d", i), Author: "bob", Updated: time.Now()})
	}
	for i := 1; i <= issues; i++ {
		d.Issues = append(d.Issues, source.Issue{Number: i, Title: fmt.Sprintf("issue %d", i), Author: "bob", Updated: time.Now()})
	}
	d.OpenPRs, d.OpenIssues = prs, issues
	return d
}

func TestChromeAndTabBar(t *testing.T) {
	m := repoModel(fakeRepo(0, 0))
	if !m.tabBarVisible() || m.chrome() != 4 {
		t.Errorf("project: tabBar=%v chrome=%d", m.tabBarVisible(), m.chrome())
	}
	m.scope = "" // All projects has no single repo, so no tabs
	if m.tabBarVisible() || m.chrome() != 3 {
		t.Errorf("all projects: tabBar=%v chrome=%d", m.tabBarVisible(), m.chrome())
	}
	m.screen = screenHome
	if m.chrome() != 3 {
		t.Errorf("home chrome = %d", m.chrome())
	}
}

func TestRepoStateMessages(t *testing.T) {
	m := repoModel(fakeRepo(0, 0))
	if d, _ := m.repoState(); d == nil {
		t.Fatal("expected data")
	}
	text := func() string { _, why := m.repoState(); return strings.Join(why, "\n") }

	m.snap.RepoRefs = nil
	if !strings.Contains(text(), "No GitHub remote") || !strings.Contains(text(), "[repos]") {
		t.Errorf("no remote: %q", text())
	}
	m = repoModel(nil)
	m.snap.Repos = nil
	m.snap.RepoErrs["/code/a"] = fmt.Errorf("boom")
	if !strings.Contains(text(), "boom") {
		t.Errorf("error: %q", text())
	}
	m = repoModel(fakeRepo(0, 0))
	m.snap.GitHubErr = source.ErrNoToken
	if !strings.Contains(text(), "gh auth login") {
		t.Errorf("no token: %q", text())
	}
	off := false
	m = repoModel(fakeRepo(0, 0))
	m.cfg.GitHub = &off
	if !strings.Contains(text(), "switched off") {
		t.Errorf("disabled: %q", text())
	}
}

func TestSubViewKeysAndScroll(t *testing.T) {
	m := repoModel(fakeRepo(60, 3))
	press := func(keys ...string) {
		for _, k := range keys {
			next, _ := m.handleRepoKey(keyMsg(k))
			m = next.(Model)
		}
	}
	press("l") // PRs
	if m.rsub != svPRs {
		t.Fatalf("rsub = %d", m.rsub)
	}
	vis := m.repoVisible()
	for i := 0; i < 55; i++ {
		press("j")
		if c, o := m.rcur[svPRs], m.roff[svPRs]; c < o || c >= o+vis {
			t.Fatalf("step %d: cursor %d outside window [%d,%d)", i, c, o, o+vis)
		}
	}
	press("l", "j", "h") // cursor per section is remembered
	if m.rcur[svPRs] != 55 || m.rcur[svIssues] != 1 {
		t.Errorf("cursors: prs=%d issues=%d", m.rcur[svPRs], m.rcur[svIssues])
	}
	press("G")
	if m.rcur[svPRs] != 59 {
		t.Errorf("G -> %d, want 59", m.rcur[svPRs])
	}
	press("l", "l") // PRs -> Issues -> Actions
	if m.rsub != svActions {
		t.Errorf("l,l from PRs -> %d, want Actions", m.rsub)
	}
	press("h", "h", "h") // back to Overview
	if m.rsub != svOverview {
		t.Errorf("h,h,h from Actions -> %d, want Overview", m.rsub)
	}
	press("h") // wraps to the last section
	if m.rsub != svAlerts {
		t.Errorf("h from Overview -> %d, want Alerts", m.rsub)
	}
}

func TestRepoTableFitsWidth(t *testing.T) {
	m := repoModel(fakeRepo(3, 3))
	for _, w := range []int{80, 100, 140, 220} {
		m.w = w
		for s := svPRs; s <= svAlerts; s++ {
			cols, rows := repoRows(m.snap.Repos["/code/a"], s)
			hdr, body := renderTable(cols, rows, w-2)
			for _, line := range append([]string{hdr}, body...) {
				if lw := lipgloss.Width(line); lw > w-2 {
					t.Errorf("width %d, section %s: line is %d wide", w, subNames[s], lw)
				}
			}
		}
	}
}

func TestAttention(t *testing.T) {
	d := fakeRepo(0, 0)
	d.DefaultCI = "FAILURE"
	d.PRs = []source.PR{
		{Repo: "o/r", Number: 1, Author: "me", CI: "FAILURE"},
		{Repo: "o/r", Number: 2, Author: "me", Review: "CHANGES_REQUESTED"},
		{Repo: "o/r", Number: 3, Author: "bob", CI: "FAILURE"}, // not yours
	}
	d.Issues = []source.Issue{{Number: 1, Assignees: []string{"me"}}, {Number: 2, Assignees: []string{"bob"}}}
	d.Alerts = []source.Alert{{Severity: "CRITICAL"}, {Severity: "LOW"}}
	m := repoModel(d)
	m.snap.GitHub.Review = []source.PR{{Repo: "o/r", Number: 3}, {Repo: "other/x", Number: 1}}

	got := strings.Join(m.attention(d), "\n")
	for _, want := range []string{"CI is failing on main", "1 of your PRs have failing CI", "1 of your PRs have changes requested",
		"1 PRs are waiting for your review", "1 issues are assigned to you", "1 critical/high security alerts"} {
		if !strings.Contains(got, want) {
			t.Errorf("attention missing %q in:\n%s", want, got)
		}
	}
	if clean := repoModel(fakeRepo(2, 2)); len(clean.attention(clean.snap.Repos["/code/a"])) != 0 {
		t.Error("a quiet repo should need no attention")
	}
}

func TestTabSwitching(t *testing.T) {
	m := repoModel(fakeRepo(1, 1))
	m.tab = tabOverview
	for _, want := range []projTab{tabDeploys, tabGitHub, tabHealth, tabOverview} { // last one wraps
		m.switchTab(1)
		if m.tab != want {
			t.Errorf("] -> %d, want %d", m.tab, want)
		}
	}
	m.scope = ""
	m.switchTab(1)
	if m.tab != tabOverview || m.flash == "" {
		t.Errorf("tabs should be refused for All projects: tab=%d flash=%q", m.tab, m.flash)
	}
}
