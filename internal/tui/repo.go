package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/browser"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/gitrepo"
	"github.com/Arnav717/oryn/internal/source"
)

// projTab is a top-level tab inside a project. New integrations (costs,
// deploys, ...) become more tabs.
type projTab int

const (
	tabDeps projTab = iota
	tabGitHub
	tabDeploys
	numTabs
)

var tabNames = [numTabs]string{"Dependencies", "GitHub", "Deploys"}

// subView is a section of the GitHub tab.
type subView int

const (
	svOverview subView = iota
	svPRs
	svIssues
	svActions
	svCommits
	svPeople
	svReleases
	svAlerts
	numSubs
)

var subNames = [numSubs]string{"Overview", "PRs", "Issues", "Actions", "Commits", "People", "Releases", "Alerts"}

// tabBarVisible: tabs only make sense for a single project.
func (m *Model) tabBarVisible() bool { return m.screen == screenProject && m.scope != "" }

// chrome is the number of screen lines used outside the body (header, footer
// and, inside a project, the tab bar).
func (m *Model) chrome() int {
	if m.tabBarVisible() {
		return 3
	}
	return 2
}

func (m *Model) switchTab(delta int) {
	if !m.tabBarVisible() {
		m.flash = "open a single project to use tabs"
		return
	}
	m.setTab(projTab((int(m.tab) + delta + int(numTabs)) % int(numTabs)))
}

func (m *Model) setTab(t projTab) {
	m.tab = t
	m.detail = ""
	if t == tabDeploys {
		m.dpCursor, m.dpOffset = 0, 0
		m.dpClamp()
	}
	m.repoClamp()
}

// repoState returns the project's repo data, or lines explaining why there is
// none.
func (m *Model) repoState() (*source.RepoData, []string) {
	why := func(lines ...string) (*source.RepoData, []string) { return nil, lines }
	switch {
	case m.snap == nil:
		return why(sDim.Render("loading…"))
	case !m.cfg.GitHubEnabled():
		return why(sDim.Render("GitHub is switched off (github = false in the config)."))
	case errors.Is(m.snap.GitHubErr, source.ErrNoToken):
		return why(
			"oryn reads GitHub through your `gh` login or a GH_TOKEN.",
			"Run "+sAccent.Render("gh auth login")+", then press "+sAccent.Render("r")+".",
		)
	}
	if _, ok := m.snap.RepoRefs[m.scope]; !ok {
		return why(
			"No GitHub remote found for this folder.",
			"",
			sDim.Render("Add one:   git remote add origin git@github.com:owner/name.git"),
			sDim.Render("Or map it in the config:"),
			sDim.Render(fmt.Sprintf("  [repos]\n  %q = \"owner/name\"", config.ShortHome(m.scope))),
		)
	}
	if err := m.snap.RepoErrs[m.scope]; err != nil {
		return why(sBad.Render(err.Error()))
	}
	if d := m.snap.Repos[m.scope]; d != nil {
		return d, nil
	}
	return why(sDim.Render("no data yet, press r to refresh"))
}

// repoRefFor is the detected GitHub repo for a project, if any.
func (m Model) repoRefFor(project string) (gitrepo.Ref, bool) {
	if m.snap == nil {
		return gitrepo.Ref{}, false
	}
	ref, ok := m.snap.RepoRefs[project]
	return ref, ok
}

// col is a list column; w == 0 takes the remaining width.
type col struct {
	title string
	w     int
}

type listRow struct {
	cells []string
	url   string
}

// repoRows builds the table for a list section.
func repoRows(d *source.RepoData, sub subView) ([]col, []listRow) {
	var rows []listRow
	switch sub {
	case svPRs:
		for _, p := range d.PRs {
			rows = append(rows, listRow{[]string{fmt.Sprintf("#%d", p.Number), p.Title, sDim.Render(p.Author), ciCell(p), reviewCell(p), sDim.Render(humanAge(p.Updated))}, p.URL})
		}
		return []col{{"#", 7}, {"TITLE", 0}, {"AUTHOR", 16}, {"CI", 4}, {"REVIEW", 14}, {"AGE", 5}}, rows
	case svIssues:
		for _, i := range d.Issues {
			rows = append(rows, listRow{[]string{fmt.Sprintf("#%d", i.Number), i.Title, sDim.Render(i.Author), sDim.Render(strings.Join(i.Labels, ", ")), sDim.Render(fmt.Sprint(i.Comments)), sDim.Render(humanAge(i.Updated))}, i.URL})
		}
		return []col{{"#", 7}, {"TITLE", 0}, {"AUTHOR", 16}, {"LABELS", 24}, {"CMT", 4}, {"AGE", 5}}, rows
	case svActions:
		for _, r := range d.Runs {
			rows = append(rows, listRow{[]string{fmt.Sprintf("#%d", r.Number), r.Workflow, r.Title, sDim.Render(r.Branch), runCell(r), sDim.Render(humanAge(r.Created))}, r.URL})
		}
		return []col{{"RUN", 7}, {"WORKFLOW", 22}, {"TITLE", 0}, {"BRANCH", 18}, {"STATUS", 12}, {"AGE", 5}}, rows
	case svCommits:
		for _, c := range d.Commits {
			sha := c.SHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			rows = append(rows, listRow{[]string{sAccent.Render(sha), c.Message, sDim.Render(c.Author), sDim.Render(humanAge(c.At))}, c.URL})
		}
		return []col{{"SHA", 9}, {"MESSAGE", 0}, {"AUTHOR", 20}, {"AGE", 5}}, rows
	case svPeople:
		for _, p := range d.People {
			role := sDim.Render("contributor")
			if p.Permission != "" {
				role = strings.ToLower(p.Permission)
			}
			commits := sDim.Render("-")
			if p.Contributions > 0 {
				commits = fmt.Sprint(p.Contributions)
			}
			rows = append(rows, listRow{[]string{p.Login, role, commits}, "https://github.com/" + p.Login})
		}
		return []col{{"LOGIN", 0}, {"ROLE", 14}, {"COMMITS", 9}}, rows
	case svReleases:
		for _, r := range d.Releases {
			name := r.Name
			if r.Prerelease {
				name += sDim.Render("  pre-release")
			}
			rows = append(rows, listRow{[]string{sAccent.Render(r.Tag), name, sDim.Render(humanAge(r.At))}, r.URL})
		}
		return []col{{"TAG", 22}, {"NAME", 0}, {"AGE", 5}}, rows
	case svAlerts:
		for _, a := range d.Alerts {
			rows = append(rows, listRow{[]string{severityBadge(a.Severity), a.Package + sDim.Render(" "+a.Ecosystem), a.Summary, sDim.Render(a.Manifest)}, a.URL})
		}
		return []col{{"SEV", 6}, {"PACKAGE", 34}, {"SUMMARY", 0}, {"MANIFEST", 24}}, rows
	}
	return nil, nil
}

func runCell(r source.Run) string {
	switch {
	case r.Status != "completed":
		return sWarn.Render("● " + strings.ReplaceAll(r.Status, "_", " "))
	case r.Conclusion == "success":
		return sOK.Render("✓ success")
	case r.Failing():
		return sBad.Render("✗ " + strings.ReplaceAll(r.Conclusion, "_", " "))
	}
	return sDim.Render(strings.ReplaceAll(r.Conclusion, "_", " "))
}

func ciWord(state string) string {
	switch state {
	case "SUCCESS":
		return sOK.Render("✓ passing")
	case "FAILURE", "ERROR":
		return sBad.Render("✗ failing")
	case "PENDING", "EXPECTED":
		return sWarn.Render("● running")
	}
	return sDim.Render("no checks")
}

// renderTable lays out cols for the given inner width and returns the header
// line plus one line per row (without cursor markers).
func renderTable(cols []col, rows []listRow, width int) (string, []string) {
	used := 2 // cursor marker
	for _, c := range cols {
		used += c.w
	}
	flex := max(10, width-used)
	var hdr strings.Builder
	hdr.WriteString("  ")
	for _, c := range cols {
		w := c.w
		if w == 0 {
			w = flex
		}
		hdr.WriteString(pad(c.title, w))
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		var b strings.Builder
		for j, cell := range r.cells {
			w := cols[j].w
			if w == 0 {
				w = flex
			}
			b.WriteString(pad(cell, w))
		}
		out[i] = b.String()
	}
	return sDim.Render(hdr.String()), out
}

func subCount(d *source.RepoData, s subView) string {
	switch s {
	case svPRs:
		return fmt.Sprint(d.OpenPRs)
	case svIssues:
		return fmt.Sprint(d.OpenIssues)
	case svPeople:
		return fmt.Sprint(len(d.People))
	case svReleases:
		return fmt.Sprint(len(d.Releases))
	case svAlerts:
		if d.AlertsKnown {
			return fmt.Sprint(d.AlertCount)
		}
	}
	return ""
}

func (m Model) subNav(d *source.RepoData) string {
	parts := make([]string, numSubs)
	for s := subView(0); s < numSubs; s++ {
		label := fmt.Sprintf("%d %s", s+1, subNames[s])
		if c := subCount(d, s); c != "" {
			label += " " + c
		}
		if s == m.rsub {
			parts[s] = sLogo.Render(label)
		} else {
			parts[s] = sDim.Render(" " + label + " ")
		}
	}
	return strings.Join(parts, "")
}

// repoVisible is how many table rows fit under the fixed top lines.
func (m *Model) repoVisible() int { return max(1, m.h-m.chrome()-2-3) }

func (m *Model) repoClamp() {
	d, _ := m.repoState()
	if d == nil {
		return
	}
	_, rows := repoRows(d, m.rsub)
	n := len(rows)
	s := m.rsub
	m.rcur[s] = max(0, min(m.rcur[s], n-1))
	vis := m.repoVisible()
	if m.rcur[s] < m.roff[s] {
		m.roff[s] = m.rcur[s]
	}
	if m.rcur[s] >= m.roff[s]+vis {
		m.roff[s] = m.rcur[s] - vis + 1
	}
	m.roff[s] = max(0, min(m.roff[s], n-vis))
}

// repoBody renders the GitHub tab.
func (m Model) repoBody() string {
	d, why := m.repoState()
	if d == nil {
		lines := append([]string{sTitle.Render("GITHUB"), ""}, why...)
		return m.fullBox(lines)
	}
	lines := []string{m.subNav(d), ""}
	if m.rsub == svOverview {
		return m.fullBox(append(lines, m.overviewLines(d)...))
	}

	cols, rows := repoRows(d, m.rsub)
	if m.rsub == svAlerts && !d.AlertsKnown {
		return m.fullBox(append(lines, sDim.Render("Dependabot alerts are only visible to repo admins (or aren't enabled for this repo).")))
	}
	if len(rows) == 0 {
		return m.fullBox(append(lines, sDim.Render("Nothing here.")))
	}
	hdr, body := renderTable(cols, rows, m.w-2)
	lines = append(lines, hdr)
	s := m.rsub
	end := min(len(body), m.roff[s]+m.repoVisible())
	for i := m.roff[s]; i < end; i++ {
		marker := "  "
		if i == m.rcur[s] {
			marker = sAccent.Render("▸ ")
		}
		lines = append(lines, marker+body[i])
	}
	if total := len(body); total < totalFor(d, m.rsub) {
		lines = append(lines, sDim.Render(fmt.Sprintf("  showing the %d most recent of %d", total, totalFor(d, m.rsub))))
	}
	return m.fullBox(lines)
}

func totalFor(d *source.RepoData, s subView) int {
	switch s {
	case svPRs:
		return d.OpenPRs
	case svIssues:
		return d.OpenIssues
	}
	return 0
}

func (m *Model) overviewLines(d *source.RepoData) []string {
	var tags []string
	if d.Private {
		tags = append(tags, "private")
	} else {
		tags = append(tags, "public")
	}
	for _, t := range []struct {
		on bool
		s  string
	}{{d.Fork, "fork"}, {d.Archived, "archived"}, {d.Language != "", d.Language}, {d.License != "", d.License}} {
		if t.on {
			tags = append(tags, t.s)
		}
	}
	lines := []string{sTitle.Render(d.Ref.String()) + "  " + sDim.Render(strings.Join(tags, " · "))}
	if d.Description != "" {
		lines = append(lines, d.Description)
	}
	lines = append(lines, "",
		fmt.Sprintf("★ %d   ⑂ %d   default branch %s   %s", d.Stars, d.Forks, sBold.Render(orDash(d.DefaultBranch)), ciWord(d.DefaultCI)),
		fmt.Sprintf("%d open PRs · %d open issues · %s", d.OpenPRs, d.OpenIssues, alertPhrase(d)),
	)
	if len(d.Releases) > 0 {
		r := d.Releases[0]
		lines = append(lines, "latest release "+sAccent.Render(r.Tag)+sDim.Render("  "+humanAge(r.At)+" ago"))
	}
	if len(d.Commits) > 0 {
		c := d.Commits[0]
		lines = append(lines, "last commit    "+c.Message+sDim.Render("  "+c.Author+", "+humanAge(c.At)+" ago"))
	}
	if len(d.Runs) > 0 {
		r := d.Runs[0]
		lines = append(lines, "last run       "+runCell(r)+sDim.Render("  "+r.Workflow+" #"+fmt.Sprint(r.Number)+", "+humanAge(r.Created)+" ago"))
	}

	lines = append(lines, "", sTitle.Render("NEEDS ATTENTION"))
	attn := m.attention(d)
	if len(attn) == 0 {
		lines = append(lines, "  "+sOK.Render("✓ nothing needs you right now"))
	}
	for _, a := range attn {
		lines = append(lines, "  "+a)
	}
	return append(lines, "", sDim.Render(d.URL+"   (enter or o opens it)"))
}

func alertPhrase(d *source.RepoData) string {
	switch {
	case !d.AlertsKnown:
		return sDim.Render("security alerts n/a")
	case d.AlertCount == 0:
		return sOK.Render("0 security alerts")
	}
	return sBad.Render(fmt.Sprintf("%d security alerts", d.AlertCount))
}

// attention lists what on this repo is waiting for the signed-in user.
func (m *Model) attention(d *source.RepoData) []string {
	var out []string
	if d.DefaultCI == "FAILURE" || d.DefaultCI == "ERROR" {
		out = append(out, sBad.Render("✗ CI is failing on "+d.DefaultBranch))
	}
	var failing, changes int
	for _, p := range d.PRs {
		if p.Author != d.Viewer {
			continue
		}
		if p.Failing() {
			failing++
		}
		if p.Review == "CHANGES_REQUESTED" {
			changes++
		}
	}
	if failing > 0 {
		out = append(out, sBad.Render(fmt.Sprintf("✗ %d of your PRs have failing CI", failing)))
	}
	if changes > 0 {
		out = append(out, sWarn.Render(fmt.Sprintf("● %d of your PRs have changes requested", changes)))
	}
	if m.snap != nil && m.snap.GitHub != nil {
		n := 0
		for _, p := range m.snap.GitHub.Review {
			if p.Repo == d.Ref.String() {
				n++
			}
		}
		if n > 0 {
			out = append(out, sAccent.Render(fmt.Sprintf("● %d PRs are waiting for your review", n)))
		}
	}
	assigned := 0
	for _, i := range d.Issues {
		for _, a := range i.Assignees {
			if a == d.Viewer {
				assigned++
			}
		}
	}
	if assigned > 0 {
		out = append(out, sAccent.Render(fmt.Sprintf("● %d issues are assigned to you", assigned)))
	}
	var crit int
	for _, a := range d.Alerts {
		if source.SeverityRank(a.Severity) >= 3 {
			crit++
		}
	}
	if crit > 0 {
		out = append(out, sBad.Render(fmt.Sprintf("▲ %d critical/high security alerts", crit)))
	}
	return out
}

func (m Model) handleRepoKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	d, _ := m.repoState()
	switch s := k.String(); s {
	case "q":
		return m, tea.Quit
	case "esc":
		m.screen = screenHome
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "]":
		m.switchTab(1)
	case "[":
		m.switchTab(-1)
	case "D":
		m.setTab(tabDeploys)
	case "l", "right", "tab":
		m.rsub = (m.rsub + 1) % numSubs
	case "h", "left", "shift+tab":
		m.rsub = (m.rsub + numSubs - 1) % numSubs
	case "1", "2", "3", "4", "5", "6", "7", "8":
		m.rsub = subView(s[0] - '1')
	case "j", "down":
		m.rcur[m.rsub]++
	case "k", "up":
		m.rcur[m.rsub]--
	case "pgdown", "ctrl+d":
		m.rcur[m.rsub] += m.repoVisible()
	case "pgup", "ctrl+u":
		m.rcur[m.rsub] -= m.repoVisible()
	case "g", "home":
		m.rcur[m.rsub] = 0
	case "G", "end":
		m.rcur[m.rsub] = 1 << 30
	case "enter":
		if d != nil {
			url := d.URL
			if m.rsub != svOverview {
				if _, rows := repoRows(d, m.rsub); m.rcur[m.rsub] >= 0 && m.rcur[m.rsub] < len(rows) {
					url = rows[m.rcur[m.rsub]].url
				}
			}
			m = m.open(url)
		}
	case "o":
		if d != nil {
			m = m.open(d.URL)
		}
	}
	m.repoClamp()
	return m, nil
}

func (m Model) open(url string) Model {
	if err := browser.Open(url); err != nil {
		m.flash = err.Error()
	} else {
		m.flash = "opened " + strings.TrimPrefix(url, "https://")
	}
	return m
}

// tabBarView is the row of tabs under the header.
func (m Model) tabBarView() string {
	var parts []string
	for t := projTab(0); t < numTabs; t++ {
		label := tabNames[t]
		if t == tabGitHub {
			if ref, ok := m.repoRefFor(m.scope); ok {
				label += " " + sDim.Render(ref.String())
			}
		}
		if t == tabDeploys {
			if names := m.deployPlatforms(m.scope); names != "" {
				label += " " + sDim.Render(names)
			}
		}
		if t == m.tab {
			parts = append(parts, sAccent.Bold(true).Underline(true).Render(tabNames[t])+strings.TrimPrefix(label, tabNames[t]))
		} else {
			parts = append(parts, sDim.Render(tabNames[t])+strings.TrimPrefix(label, tabNames[t]))
		}
	}
	return " " + strings.Join(parts, sDim.Render("   │   ")) + sDim.Render("      [ ] switch tab")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// repoSummary is the compact one-liner used on the home screen's project rows.
func (m Model) repoCell(project string) string {
	if m.snap == nil {
		return sDim.Render("…")
	}
	if _, ok := m.snap.RepoRefs[project]; !ok {
		return sDim.Render("-")
	}
	if m.snap.RepoErrs[project] != nil {
		return sBad.Render("error")
	}
	d := m.snap.Repos[project]
	if d == nil {
		return sDim.Render("…")
	}
	cell := fmt.Sprintf("%d PR · %d iss", d.OpenPRs, d.OpenIssues)
	if d.DefaultCI == "FAILURE" || d.DefaultCI == "ERROR" {
		return sBad.Render("✗ ") + cell
	}
	if d.OpenPRs+d.OpenIssues == 0 {
		return sDim.Render(cell)
	}
	return cell
}

// repoData is the fetched repo for a project, nil-safe on the snapshot.
func (m Model) repoDataFor(project string) *source.RepoData {
	if m.snap == nil {
		return nil
	}
	return m.snap.Repos[project]
}
