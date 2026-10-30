package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/deploy"
	"github.com/Arnav717/oryn/internal/gitrepo"
	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
)

func busyModel() Model {
	now := time.Now()
	d := fakeRepo(0, 0)
	d.DefaultCI = "FAILURE"
	d.Alerts = []source.Alert{{Severity: "CRITICAL"}} // a known alert: Health's job
	m := shellModel()
	m.now, m.since = now, now.Add(-time.Hour)
	m.snap = &app.Snapshot{
		RepoRefs: map[string]gitrepo.Ref{"/code/a": {Owner: "o", Name: "r"}},
		Repos:    map[string]*source.RepoData{"/code/a": d},
		GitHub: &source.GitHubData{Login: "me", Review: []source.PR{
			{Repo: "o/r", Number: 1, Title: "watched", URL: "https://x/1"},
			{Repo: "other/x", Number: 2, Title: "elsewhere", URL: "https://x/2"},
		}},
		Deploys: []app.DeployTarget{{
			Link: deploy.Link{Platform: "vercel", Project: "/code/b", Ref: "prj"}, Queryable: true,
			Deployments: []deploy.Deployment{{Target: "production", State: deploy.StateError, Created: now.Add(-time.Hour), URL: "https://b.dev"}},
		}},
		Vulns: map[string][]source.Vuln{"k": {{ID: "CVE-1", Severity: "CRITICAL", Package: "lodash"}}},
	}
	m.index()
	return m
}

func itemTexts(items []attnItem) string {
	var out []string
	for _, it := range items {
		out = append(out, it.text)
	}
	return strings.Join(out, "\n")
}

func TestNeedsYou(t *testing.T) {
	m := busyModel()
	items := m.needsYou()
	got := itemTexts(items)
	for _, want := range []string{"CI is failing on main", "vercel deploy failed", "1 PRs are waiting for your review"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "alerts") || strings.Contains(got, "CVE") || strings.Contains(got, "vulnerabilit") {
		t.Errorf("known health items must stay out of the queue:\n%s", got)
	}
	// One review in the watched repo, one elsewhere: each counted once.
	if n := strings.Count(got, "waiting for your review"); n != 2 {
		t.Errorf("review rows = %d:\n%s", n, got)
	}
	for i := 1; i < len(items); i++ {
		if items[i].sev < items[i-1].sev {
			t.Errorf("not ranked:\n%s", got)
		}
	}
	if b := itemTexts(m.needsFor("/code/b")); b != "vercel deploy failed" {
		t.Errorf("needsFor b = %q", b)
	}
}

func TestNewCriticalReachesTheQueue(t *testing.T) {
	m := busyModel()
	m.snap.Events = []store.Event{{Key: "v", Kind: "vuln", Severity: 4, At: m.now}}
	if got := itemTexts(m.needsYou()); !strings.Contains(got, "1 new critical vulnerabilities") {
		t.Errorf("queue:\n%s", got)
	}
}

func TestUpstreamOutageStaysOnHealth(t *testing.T) {
	m := busyModel()
	m.snap.Status = []source.Status{{Service: "Cloudflare", Indicator: "major", Description: "Major Outage"}}
	if got := itemTexts(m.needsYou()); strings.Contains(got, "Cloudflare") {
		t.Errorf("queue:\n%s", got)
	}
}

func TestHomeIsMinimal(t *testing.T) {
	m := busyModel()
	m.snap.Events = []store.Event{{Key: "o", Kind: "pr", Entity: "o/r", Title: "x", At: m.now}}
	out := m.View()
	for _, want := range []string{"Needs you", "Deploys", "GitHub"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, not := range []string{"lodash", "CVE-1", "vulns", "outdated", "project(s)", "Not built", "updated"} {
		if strings.Contains(out, not) {
			t.Errorf("home should not show %q:\n%s", not, out)
		}
	}
	first, _, _ := strings.Cut(out, "\n")
	if strings.ContainsAny(first, "0123456789") && !strings.Contains(first, ":") {
		t.Errorf("tab bar carries counts: %q", first)
	}
	for _, s := range sections {
		if !strings.Contains(first, s.title) {
			t.Errorf("tab %s missing from %q", s.title, first)
		}
	}
}

func TestHomeLayoutFitsSize(t *testing.T) {
	m := busyModel()
	m.w, m.h = 120, 30
	if out := m.homeBody(); !strings.Contains(out, "GitHub") || !strings.Contains(out, "Needs you") {
		t.Errorf("wide layout:\n%s", out)
	}
	m.w, m.h = 80, 40 // stacks
	if out := m.homeBody(); !strings.Contains(out, "GitHub") || !strings.Contains(out, "Needs you") {
		t.Errorf("stacked layout:\n%s", out)
	}
	m.w, m.h = 80, 14 // too short to stack: only the focused window
	m.win = winDeploys
	out := m.homeBody()
	if !strings.Contains(out, "Deploys") || strings.Contains(out, "Needs you") {
		t.Errorf("short layout:\n%s", out)
	}
}

func TestWindowKeysAndEnter(t *testing.T) {
	m := busyModel()
	if got := press(t, m, "tab", "tab"); got.win != winGitHub {
		t.Errorf("tab,tab -> window %d", got.win)
	}
	if got := press(t, m, "shift+tab"); got.win != winGitHub {
		t.Errorf("shift+tab wraps -> window %d", got.win)
	}
	got := press(t, m, "tab", "enter")
	if got.screen != screenProject || got.scope != "/code/b" || got.tab != tabDeploys {
		t.Errorf("enter on deploy row -> screen %d scope %q tab %d", got.screen, got.scope, got.tab)
	}

	// The first queue row is a failure; enter lands on the tab that explains it.
	rows := needsRows(&m)
	got = press(t, m, "enter")
	if rows[0].to.project == "" || got.scope != rows[0].to.project {
		t.Errorf("enter on queue row: %+v -> scope %q", rows[0].to, got.scope)
	}

	// The cursor stops at both ends and survives a shrinking list.
	c := press(t, m, "j", "j", "j", "j", "j", "j", "j", "j")
	if c.winCur[winNeeds] != len(rows)-1 {
		t.Errorf("cursor %d of %d rows", c.winCur[winNeeds], len(rows))
	}
	c.snap = &app.Snapshot{}
	c.index()
	if got := press(t, c, "enter"); got.screen != screenHome {
		t.Errorf("enter on the empty queue -> screen %d", got.screen)
	}
}

func TestEmptyHome(t *testing.T) {
	m := testModel("/code/new")
	m.w, m.h = 120, 30
	if out := m.View(); !strings.Contains(out, "Press 2 to add a project") {
		t.Errorf("no projects:\n%s", out)
	}
	if got := press(t, m, "enter"); got.screen != screenProjects {
		t.Errorf("enter with no projects -> screen %d", got.screen)
	}
}

func TestProjectOpensOnOverview(t *testing.T) {
	m := busyModel()
	m.openScope("/code/a")
	if m.tab != tabOverview {
		t.Fatalf("project opened on tab %d", m.tab)
	}
	out := m.View()
	for _, want := range []string{"Needs", "CI is failing on main", "Live", "not deployed yet", "GitHub"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "vercel deploy failed") {
		t.Errorf("another project's failure leaked in:\n%s", out)
	}

	m.openScope("/code/b")
	if out := m.View(); !strings.Contains(out, "https://b.dev") || !strings.Contains(out, "vercel deploy failed") {
		t.Errorf("b overview:\n%s", out)
	}
}

func TestOverviewEnterOpensTheTab(t *testing.T) {
	m := busyModel()
	m.openScope("/code/a")
	rows := m.overviewRows()
	for i, r := range rows {
		if r.to.url != "" {
			continue
		}
		m.ovCur = i
		got := press(t, m, "enter")
		if got.tab != r.to.tab {
			t.Errorf("row %d -> tab %d, want %d", i, got.tab, r.to.tab)
		}
	}
	first := press(t, m, "]")
	if first.tab != tabDeploys {
		t.Errorf("] from overview -> %d", first.tab)
	}
}

func TestOverviewDeployKey(t *testing.T) {
	m := busyModel()
	m.openScope("/code/a") // nothing linked
	got := press(t, m, "p")
	if got.tab != tabDeploys || !strings.Contains(got.flash, "link a platform first") {
		t.Errorf("no link: tab %d flash %q", got.tab, got.flash)
	}

	m = busyModel()
	m.snap.Deploys = append(m.snap.Deploys, app.DeployTarget{Link: deploy.Link{Platform: "netlify", Project: "/code/b", Ref: "site"}})
	m.openScope("/code/b")
	got = press(t, m, "p")
	if got.deployAsk != nil || got.tab != tabDeploys || !strings.Contains(got.flash, "vercel and netlify") {
		t.Errorf("two links: ask %v tab %d flash %q", got.deployAsk, got.tab, got.flash)
	}
}

func TestLiveRowShowsThePublicAddress(t *testing.T) {
	m := busyModel()
	m.snap.Deploys[0].LiveURL = "https://example.com"
	cell := m.liveCell(m.snap.Deploys[0])
	if !strings.Contains(cell, "https://example.com") || strings.Contains(cell, "https://b.dev") {
		t.Errorf("cell: %q", cell)
	}
	m.snap.Deploys[0].LiveURL = ""
	if cell := m.liveCell(m.snap.Deploys[0]); !strings.Contains(cell, "https://b.dev") {
		t.Errorf("no public address known should fall back to the deployment: %q", cell)
	}
}
