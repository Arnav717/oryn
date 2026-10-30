package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Arnav717/oryn/internal/detect"
	"github.com/Arnav717/oryn/internal/store"
)

type size struct{ w, h int }

// paneSize returns the outer size (including border) of a pane.
// Layout: portfolio | vulns on top, upstream | tape below.
func (m *Model) paneSize(p paneID) size {
	gridH := m.h - m.chrome() // header, footer and (in a project) the tab bar
	topH := gridH / 2
	botH := gridH - topH
	switch p {
	case pPortfolio:
		return size{m.w / 2, topH}
	case pVulns:
		return size{m.w - m.w/2, topH}
	case pStatus:
		return size{m.w / 3, botH}
	default:
		return size{m.w - m.w/3, botH}
	}
}

// visible is the number of data rows a pane can show (border + 2 header lines).
func (m *Model) visible(p paneID) int { return m.paneSize(p).h - 4 }

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.w < 60 || m.h < 14 {
		return "terminal too small (need 60x14)"
	}
	var body string
	switch {
	case m.showHelp:
		body = m.fullBox(helpLines())
	case m.screen == screenHome:
		body = m.fullBox(m.homeLines())
	case m.screen == screenChanges:
		lines, _ := m.changesLines()
		body = m.fullBox(lines[min(m.chOffset, len(lines)):])
	case m.screen == screenGitHub:
		lines, _, _ := m.ghLines()
		body = m.fullBox(lines[min(m.ghOffset, len(lines)):])
	case m.detail != "":
		body = m.fullBox(m.detailLines())
	case m.tabBarVisible() && m.tab == tabGitHub:
		body = m.repoBody()
	default:
		top := lipgloss.JoinHorizontal(lipgloss.Top, m.pane(pPortfolio), m.pane(pVulns))
		bot := lipgloss.JoinHorizontal(lipgloss.Top, m.pane(pStatus), m.pane(pTape))
		body = lipgloss.JoinVertical(lipgloss.Left, top, bot)
	}
	if m.tabBarVisible() && !m.showHelp {
		return lipgloss.JoinVertical(lipgloss.Left, m.header(), m.tabBarView(), body, m.footer())
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.header(), body, m.footer())
}

func (m Model) header() string {
	left := sLogo.Render("ORYN")
	if m.screen == screenChanges {
		left += " " + sDim.Render("›") + " " + sBold.Render("changes")
		if !m.since.IsZero() {
			left += "  " + sDim.Render(fmt.Sprintf("since %s ago · %d new", shortAgo(m.now.Sub(m.since)), len(m.sinceEvents())))
		}
	} else if m.screen == screenGitHub {
		left += " " + sDim.Render("›") + " " + sBold.Render("github")
		if review, mine, failing := m.ghSummary(); m.snap != nil && m.snap.GitHub != nil {
			parts := []string{fmt.Sprintf("%d to review", review), fmt.Sprintf("%d open", mine)}
			if failing > 0 {
				parts = append(parts, sBad.Render(fmt.Sprintf("%d failing CI", failing)))
			}
			left += "  " + strings.Join(parts, sDim.Render(" · "))
		}
	} else if m.screen == screenHome {
		left += " " + sDim.Render("home")
		if m.snap != nil {
			left += "  " + strings.Join([]string{
				fmt.Sprintf("%d project(s)", len(m.cfg.Projects)),
				vulnSummary(m.total.Vulns, m.total.Crit, m.total.High),
				sWarn.Render(fmt.Sprintf("%d outdated", m.total.Outdated)),
				m.upstreamPart(),
			}, sDim.Render(" · "))
		}
	} else {
		name := "all projects"
		if m.scope != "" {
			name = filepath.Base(m.scope)
		}
		left += " " + sDim.Render("›") + " " + sBold.Render(name)
		if d := m.repoDataFor(m.scope); m.tabBarVisible() && m.tab == tabGitHub && d != nil {
			left += "  " + strings.Join([]string{
				fmt.Sprintf("★ %d", d.Stars),
				fmt.Sprintf("%d PRs", d.OpenPRs),
				fmt.Sprintf("%d issues", d.OpenIssues),
				ciWord(d.DefaultCI),
				alertPhrase(d),
			}, sDim.Render(" · "))
		} else if m.snap != nil {
			var crit, high, outdated int
			for _, v := range m.vulns {
				switch v.Severity {
				case "CRITICAL":
					crit++
				case "HIGH":
					high++
				}
			}
			for _, p := range m.pkgs {
				if p.Direct && p.outdated() {
					outdated++
				}
			}
			left += "  " + strings.Join([]string{
				fmt.Sprintf("%d pkgs", len(m.pkgs)),
				vulnSummary(len(m.vulns), crit, high),
				sWarn.Render(fmt.Sprintf("%d outdated", outdated)),
				m.upstreamPart(),
			}, sDim.Render(" · "))
		}
	}
	right := m.now.Format("Mon 02 Jan 15:04:05")
	if m.loading {
		right = m.spin.View() + " refreshing  " + right
	}
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return clip(left, m.w)
	}
	return left + strings.Repeat(" ", gap) + sAccent.Render(right)
}

func vulnSummary(total, crit, high int) string {
	if total == 0 {
		return sOK.Render("0 vulns")
	}
	s := fmt.Sprintf("%d vulns", total)
	if crit > 0 {
		s += fmt.Sprintf(" (%d crit, %d high)", crit, high)
		return sBad.Bold(true).Render(s)
	}
	if high > 0 {
		s += fmt.Sprintf(" (%d high)", high)
	}
	return lipgloss.NewStyle().Foreground(orange).Render(s)
}

func (m Model) footer() string {
	if m.cmdMode {
		return clip(m.input.View()+m.suggestions(), m.w)
	}
	if m.screen == screenChanges {
		hints := sDim.Render("j/k move · enter open in browser · e expand/fold · esc home · r refresh · ? help · q quit")
		if m.flash == "" {
			return clip(hints, m.w)
		}
		return clip(sAccent.Render(m.flash)+sDim.Render("  │  ")+hints, m.w)
	}
	if m.screen == screenGitHub {
		hints := sDim.Render("j/k move · enter open in browser · esc home · r refresh · ? help · q quit")
		if m.flash == "" {
			return clip(hints, m.w)
		}
		return clip(sAccent.Render(m.flash)+sDim.Render("  │  ")+hints, m.w)
	}
	if m.screen == screenHome {
		switch items := m.homeItems(); {
		case m.adding:
			return clip(m.pathInput.View(), m.w)
		case m.confirmRm && m.homeCursor < len(items):
			return sWarn.Render("stop watching " + filepath.Base(items[m.homeCursor].path) + "?  y to confirm, any other key cancels")
		}
		hints := sDim.Render("j/k move · enter open · 1-9 jump · a add · d remove · r refresh · ? help · q quit")
		if m.flash == "" {
			return clip(hints, m.w)
		}
		return clip(sAccent.Render(m.flash)+sDim.Render("  │  ")+hints, m.w)
	}
	hints := sDim.Render("tab pane · j/k move · enter open · / search · esc home · r refresh · ? help · q quit")
	if m.tabBarVisible() {
		hints = sDim.Render("tab pane · j/k move · enter open · / search · [ ] tabs · esc home · r refresh · ? help · q quit")
		if m.tab == tabGitHub {
			hints = sDim.Render("h/l section · 1-8 jump · j/k move · enter open · o repo · [ ] tabs · esc home · r refresh · ? help · q quit")
		}
	}
	if m.flash == "" {
		return clip(hints, m.w)
	}
	return clip(sAccent.Render(m.flash)+sDim.Render("  │  ")+hints, m.w)
}

// suggestions lists live package matches for the command input; the first is
// what enter opens and tab completes.
func (m Model) suggestions() string {
	q := strings.TrimSpace(m.input.Value())
	if _, ok := aliases[strings.ToLower(q)]; ok || len(q) == 0 {
		return ""
	}
	if _, ok := commands[strings.ToLower(q)]; ok {
		return ""
	}
	keys := m.matches(q, 6)
	if len(keys) == 0 {
		return sDim.Render("   no match")
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		p := m.pkgs[k]
		label := p.Name
		if n := len(p.Vulns); n > 0 {
			label += rankStyle(p.maxSeverity()).Render(fmt.Sprintf(" ▲%d", n))
		}
		if i == 0 {
			parts[i] = sAccent.Render("↵ ") + sBold.Render(p.Name) + strings.TrimPrefix(label, p.Name)
		} else {
			parts[i] = sDim.Render(label)
		}
	}
	return "   " + strings.Join(parts, sDim.Render("  ·  "))
}

func (m Model) pane(p paneID) string {
	sz := m.paneSize(p)
	cw, ch := sz.w-2, sz.h-2
	rowW := cw - 2 // room for the cursor marker

	sub, rows := m.paneContent(p, rowW)
	lines := []string{
		sTitle.Render(fmt.Sprintf("%d %s", p+1, paneTitles[p])) + sDim.Render(fmt.Sprintf("  %d", m.rowCount(p))),
		"  " + sDim.Render(clip(sub, rowW)),
	}
	if m.snap == nil {
		lines = append(lines, "  "+sDim.Render("loading…"))
	}
	end := min(len(rows), m.offset[p]+m.visible(p))
	for i := m.offset[p]; i < end; i++ {
		marker := "  "
		if i == m.cursor[p] && p == m.focus {
			marker = sAccent.Render("▸ ")
		}
		lines = append(lines, marker+pad(rows[i], rowW))
	}
	for i := range lines {
		lines[i] = clip(lines[i], cw)
	}
	if len(lines) > ch {
		lines = lines[:ch]
	}
	return paneBox(p == m.focus).Width(cw).Height(ch).Render(strings.Join(lines, "\n"))
}

// paneContent returns a column/summary line and the rows.
func (m Model) paneContent(p paneID, w int) (string, []string) {
	if m.snap == nil {
		return "", nil
	}
	switch p {
	case pPortfolio:
		return m.portfolioRows(w)
	case pVulns:
		return m.vulnRows(w)
	case pStatus:
		return m.statusRows(w)
	default:
		return m.tapeRows(w)
	}
}

func (m Model) portfolioRows(w int) (string, []string) {
	const verW, latW, vulW = 14, 14, 6
	nameW := max(8, w-verW-latW-vulW)
	sub := pad("PACKAGE", nameW) + pad("VERSION", verW) + pad("LATEST", latW) + "VULNS"
	rows := make([]string, 0, len(m.pkgOrder))
	for _, k := range m.pkgOrder {
		p := m.pkgs[k]
		name := p.Name
		if !p.Direct {
			name = sDim.Render("↳ ") + name
		}
		if p.DevOnly {
			name += sDim.Render(" dev")
		}
		ver := ""
		if len(p.Versions) > 0 {
			ver = p.Versions[0]
			if len(p.Versions) > 1 {
				ver += fmt.Sprintf(" +%d", len(p.Versions)-1)
			}
		}
		latest := sDim.Render("-")
		switch {
		case p.outdated():
			latest = sWarn.Render(p.Latest)
		case p.Latest != "":
			latest = sOK.Render("✓")
		}
		vul := sDim.Render("·")
		if n := len(p.Vulns); n > 0 {
			vul = rankStyle(p.maxSeverity()).Render(fmt.Sprintf("▲ %d", n))
		}
		rows = append(rows, pad(name, nameW)+pad(ver, verW)+pad(latest, latW)+vul)
	}
	return sub, rows
}

func (m Model) vulnRows(w int) (string, []string) {
	pkgW := min(28, max(12, w/3))
	const idW, fixW = 20, 12
	sub := pad("SEV", 5) + pad("PACKAGE", pkgW) + pad("ID", idW) + pad("FIX", fixW) + "SUMMARY"
	rows := make([]string, 0, len(m.vulns))
	for _, v := range m.vulns {
		fix := sDim.Render("none")
		if v.Fixed != "" {
			fix = sOK.Render(v.Fixed)
		}
		rows = append(rows, severityBadge(v.Severity)+" "+
			pad(v.Package+"@"+v.Version, pkgW-1)+" "+pad(v.CVE(), idW)+pad(fix, fixW)+v.Summary)
	}
	return sub, rows
}

func (m Model) statusRows(w int) (string, []string) {
	issues := 0
	rows := make([]string, 0, len(m.snap.Status))
	for _, s := range m.snap.Status {
		dot, desc := sOK.Render("●"), s.Description
		switch {
		case s.Err != nil:
			dot, desc = sDim.Render("?"), sDim.Render("unreachable")
		case s.Indicator == "minor" || s.Indicator == "maintenance":
			dot, desc = sWarn.Render("●"), sWarn.Render(desc)
		case s.Indicator == "major" || s.Indicator == "critical":
			dot, desc = sBad.Render("●"), sBad.Render(desc)
		}
		if !s.OK() {
			issues++
		}
		if s.Via != "" {
			desc += sDim.Render("  ← " + s.Via)
		}
		rows = append(rows, dot+" "+pad(s.Service, 13)+" "+desc)
	}
	return fmt.Sprintf("%d services · %d with issues", len(rows), issues), rows
}

func (m Model) tapeRows(w int) (string, []string) {
	rows := make([]string, 0, len(m.events))
	for _, e := range m.events {
		rows = append(rows, eventLine(e))
	}
	return "newest first · new vulns, releases and status changes", rows
}

func eventLine(e store.Event) string {
	st := rankStyle(e.Severity)
	switch {
	case e.Kind == "release":
		st = lipgloss.NewStyle().Foreground(blue)
	case e.Kind == "status" && e.Severity == 0:
		st = sOK
	}
	return sDim.Render(e.At.Format("Jan02 15:04")) + " " + st.Render(pad(strings.ToUpper(e.Kind), 7)) + " " + e.Title
}

func (m Model) fullBox(lines []string) string {
	cw, ch := m.w-2, m.h-m.chrome()-2
	if m.detail != "" {
		m.detailOff = min(m.detailOff, max(0, len(lines)-ch))
		lines = lines[m.detailOff:]
	}
	if len(lines) > ch {
		lines = lines[:ch]
	}
	for i := range lines {
		lines[i] = clip(lines[i], cw)
	}
	return paneBox(true).Width(cw).Height(ch).Render(strings.Join(lines, "\n"))
}

func (m Model) detailLines() []string {
	p := m.pkgs[m.detail]
	if p == nil {
		return []string{sDim.Render("package not found · esc to go back")}
	}
	kind := "direct"
	if !p.Direct {
		kind = "transitive"
	}
	if p.DevOnly {
		kind += " · dev"
	}
	lines := []string{
		sTitle.Render(p.Name) + "  " + sDim.Render(p.Eco+" · "+kind) + sDim.Render("   DES · esc back · j/k scroll"),
		"",
		pad(sDim.Render("your version"), 16) + strings.Join(p.Versions, ", "),
	}
	latest := sDim.Render("unknown (only checked for direct deps)")
	if p.Latest != "" {
		latest = sOK.Render(p.Latest + "  up to date")
		if p.outdated() {
			latest = sWarn.Render(p.Latest + "  ↑ update available")
		}
	}
	lines = append(lines,
		pad(sDim.Render("latest"), 16)+latest,
		pad(sDim.Render("used in"), 16)+strings.Join(p.Projects, ", "),
		pad(sDim.Render("registry"), 16)+registryURL(p.Eco, p.Name),
		"",
	)

	if len(p.Vulns) == 0 {
		return append(lines, sOK.Render("No known vulnerabilities."))
	}
	lines = append(lines, sTitle.Render(fmt.Sprintf("VULNERABILITIES  %d", len(p.Vulns))), "")
	for _, v := range p.Vulns {
		fix := sBad.Render("no fix released")
		if v.Fixed != "" {
			fix = sOK.Render("fixed in " + v.Fixed)
		}
		lines = append(lines,
			severityBadge(v.Severity)+"  "+sBold.Render(v.CVE())+sDim.Render("  "+v.ID+"  affects "+v.Version+"  ")+fix,
			"      "+v.Summary,
			"      "+sDim.Render(v.URL),
			"",
		)
	}
	return lines
}

func registryURL(eco, name string) string {
	switch eco {
	case detect.NPM:
		return "https://www.npmjs.com/package/" + name
	case detect.PyPI:
		return "https://pypi.org/project/" + name + "/"
	case detect.Go:
		return "https://pkg.go.dev/" + name
	}
	return ""
}

func helpLines() []string {
	row := func(k, d string) string { return "  " + sAccent.Render(pad(k, 22)) + d }
	return []string{
		sTitle.Render("ORYN HELP") + sDim.Render("   any key to close"),
		"",
		sBold.Render("Navigation"),
		row("tab / shift+tab, h/l", "next / previous pane"),
		row("1 2 3 4", "jump to pane"),
		row("j/k, ↑/↓", "move"),
		row("g / G, pgup / pgdn", "top / bottom, page"),
		row("enter", "open package (DES)"),
		row("c (on home)", "changes since you last looked"),
		row("r", "refresh now"),
		row("q", "quit"),
		"",
		sBold.Render("Home") + sDim.Render("  (esc from a project)"),
		row("j/k, enter", "pick a project (1-9 jumps straight in)"),
		row("a", "add a project directory (prefilled with where you launched oryn)"),
		row("d", "stop watching the selected project"),
		"",
		sBold.Render("GitHub inbox") + sDim.Render("  (open it from home)"),
		row("j/k, enter", "move · open the PR in your browser"),
		row("esc", "back to home"),
		"",
		sBold.Render("Project tabs") + sDim.Render("  (inside a single project)"),
		row("[  ]", "previous / next tab (Dependencies, GitHub)"),
		row("h/l or 1-8", "GitHub: Overview, PRs, Issues, Actions, Commits, People, Releases, Alerts"),
		row("enter / o", "GitHub: open the selected row / the repo in your browser"),
		row("/g  /d", "jump to the GitHub / Dependencies tab"),
		"",
		sBold.Render("Search") + sDim.Render("  (press / or :, inside a project)"),
		row("any letters", "fuzzy-find a package, e.g. `lod`, `chi`"),
		row("tab", "complete best match"),
		row("enter", "open best match"),
		row("h  p  v  s  t", "home · portfolio · vulns · status · tape"),
		row("r  ?  q", "refresh · help · quit"),
		"",
		sDim.Render("Config: ~/.config/oryn/config.toml · data: ~/.local/share/oryn/oryn.db"),
	}
}
