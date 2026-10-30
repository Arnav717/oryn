package tui

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/detect"
	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
)

type size struct{ w, h int }

// paneSize returns the outer size (including border) of a pane.
// Layout: packages | vulns on top, upstream across the bottom.
func (m *Model) paneSize(p paneID) size {
	gridH := m.h - m.chrome() // header, footer and (in a project) the tab bar
	botH := max(5, gridH/3)
	topH := gridH - botH
	switch p {
	case pPackages:
		return size{m.w / 2, topH}
	case pVulns:
		return size{m.w - m.w/2, topH}
	}
	return size{m.w, botH}
}

// visible is the number of data rows a pane can show (border + column header).
func (m *Model) visible(p paneID) int { return m.paneSize(p).h - 3 }

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.w < 60 || m.h < 14 {
		return "terminal too small (need 60x14)"
	}
	rows := []string{m.tabsLine()}
	if m.hasContext() {
		rows = append(rows, m.context())
	}
	if m.tabBarVisible() && !m.showHelp && !m.palOpen {
		rows = append(rows, m.tabBarView())
	}
	out := strings.Split(lipgloss.JoinVertical(lipgloss.Left, append(rows, m.body(), m.footer())...), "\n")
	for i := range out {
		out[i] = paint(out[i], bgColor, m.w)
	}
	return strings.Join(out, "\n")
}

func (m Model) body() string {
	switch {
	case m.palOpen:
		return m.paletteBody()
	case m.spawn != nil:
		return m.fullBox(m.spawnLines())
	case m.showHelp:
		return m.fullBox(helpLines())
	case m.screen == screenDeployRun:
		return m.fullBox(m.runLines())
	case m.screen == screenAccounts:
		return m.fullBox(m.acctLines())
	case m.screen == screenPicker:
		return m.fullBox(m.pickLinesScrolled())
	case m.detail != "":
		return m.fullBox(m.detailLines())
	case m.screen == screenProject && m.tab == tabOverview:
		return m.overviewBody()
	case m.screen == screenProject && m.tab == tabAgents:
		return m.fullBox(m.agentBody())
	case m.screen == screenProject && m.tab == tabDeploys:
		return m.fullBox(m.deployBody())
	case m.screen == screenProject && m.tab == tabGitHub:
		return m.repoBody()
	case m.screen == screenProject && m.tab == tabActivity:
		return m.activityBody()
	case m.screen == screenProject:
		return m.panes()
	}
	return sections[m.section()].body(m)
}

// panes is the Health view.
func (m Model) panes() string {
	top := lipgloss.JoinHorizontal(lipgloss.Top, m.pane(pPackages), m.pane(pVulns))
	return lipgloss.JoinVertical(lipgloss.Left, top, m.pane(pStatus))
}

// hasContext: most screens say enough on their own. Sub-screens need a
// breadcrumb, and Health is where its numbers belong.
func (m Model) hasContext() bool {
	switch m.screen {
	case screenDeployRun, screenAccounts, screenPicker, screenHealth:
		return true
	}
	return m.screen == screenProject && m.tab == tabHealth
}

// context is the optional line under the tabs.
func (m Model) context() string {
	crumb := func(s string) string { return sDim.Render("›") + " " + sBold.Render(s) }
	var left string
	switch m.screen {
	case screenDeployRun:
		left = crumb("deploy") + "  " + sDim.Render(filepath.Base(m.run.project))
	case screenAccounts:
		left = crumb("accounts")
	case screenPicker:
		left = crumb("link project")
	case screenHealth, screenProject:
		if m.snap == nil {
			break
		}
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
		left = strings.Join([]string{
			sDim.Render(fmt.Sprintf("%d packages", len(m.pkgs))),
			vulnSummary(len(m.vulns), crit, high),
			sDim.Render(fmt.Sprintf("%d outdated", outdated)),
			m.upstreamPart(),
		}, sDim.Render(" · "))
	}
	return clip(" "+left, m.w)
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
	if m.spawn != nil {
		return m.statusLine(m.spawn.hints())
	}
	if m.stopAsk != nil {
		return clip(" "+sWarn.Render("stop "+m.stopAsk.Slug+"? its work stays on "+m.stopAsk.Branch+".  y to confirm, any other key cancels"), m.w)
	}
	if m.delAsk != nil {
		return clip(m.deletePrompt(), m.w)
	}
	if m.deployAsk != nil {
		prompt := sWarn.Render("run: " + m.deployAsk.Display() + " ?  y to confirm, any other key cancels")
		if len(m.deployAsk.Warn) > 0 {
			prompt = sBad.Render("WARNING: "+strings.Join(m.deployAsk.Warn, "; ")+".  ") + prompt
		}
		return clip(prompt, m.w)
	}
	if m.screen == screenDeployRun && m.run != nil {
		hints := "enter back · q quit"
		if m.run.url != "" && m.run.err == nil {
			hints = "enter back · o open site · q quit"
		}
		if m.run.running() {
			hints = "x cancel deploy · ctrl+c quit"
		}
		if m.flash == "" {
			return m.statusLine(hints)
		}
		return m.statusLine(hints)
	}
	if m.screen == screenPicker {
		switch {
		case m.createConfirm:
			return clip(sWarn.Render("create project '"+m.createName+"' on "+m.pickTarget.Link.Platform+"?  y to confirm, any other key cancels"), m.w)
		case m.creating:
			return clip(m.createInput.View()+sDim.Render("   enter to continue · esc cancel"), m.w)
		}
	}
	if m.cmdMode {
		return clip(m.input.View()+m.suggestions(), m.w)
	}
	if m.screen == screenAccounts {
		if m.tokenEntry {
			return clip(m.tokenInput.View()+sDim.Render("   enter to verify · esc cancel"), m.w)
		}
		return m.statusLine("enter paste token · b token page · x forget · esc back")
	}
	if m.screen == screenPicker {
		return m.statusLine("enter link · n new project · esc back")
	}
	if m.screen == screenProjects {
		items := m.homeItems()
		switch {
		case m.adding:
			return clip(m.pathInput.View(), m.w)
		case m.confirmRm && m.homeCursor < len(items):
			return sWarn.Render("stop watching " + filepath.Base(items[m.homeCursor].path) + "?  y to confirm, any other key cancels")
		}
	}
	hints := sections[m.section()].hints
	if m.screen == screenProject {
		hints = m.projectHints()
	}
	if m.agOpen != "" && (m.screen == screenAgents || m.tab == tabAgents) {
		hints = "a attach · x stop · esc back"
	}
	return m.statusLine(hints)
}

// statusLine is the bottom row: where you are, or the latest message, on the
// left and the keys on the right.
func (m Model) statusLine(hints string) string {
	left := sDim.Render(" " + config.ShortHome(orDefault(m.scope, m.cwd)))
	if m.flash != "" {
		left = " " + sAccent.Render(m.flash)
	}
	parts := strings.Split(hints, " · ")
	for i, h := range parts {
		cut := strings.LastIndex(h, " ")
		parts[i] = sBold.Render(h[:max(cut, 0)]) + sDim.Render(h[max(cut, 0):])
	}
	right := strings.Join(parts, "  ") + " "
	if gap := m.w - lipgloss.Width(left) - lipgloss.Width(right); gap > 0 {
		return left + strings.Repeat(" ", gap) + right
	}
	if m.flash != "" {
		return clip(left, m.w)
	}
	return clip(" "+right, m.w)
}

func (m Model) projectHints() string {
	switch m.tab {
	case tabAgents:
		return "n new · a attach · x stop · enter details · [ ] tabs"
	case tabOverview, tabDeploys:
		return "enter open · p deploy · [ ] tabs · ctrl+p commands"
	case tabGitHub:
		return "h/l section · enter open · [ ] tabs · ctrl+p commands"
	case tabActivity:
		return "f filter · enter open · [ ] tabs · ctrl+p commands"
	}
	return "tab pane · / search · [ ] tabs · ctrl+p commands"
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
	cw, ch := sz.w-4, sz.h-2
	rowW := cw - 2 // room for the cursor marker

	sub, rows := m.paneContent(p, rowW)
	lines := []string{
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
	return panel(paneTitles[p], lines, sz.w, sz.h, p == m.focus)
}

// paneContent returns a column/summary line and the rows.
func (m Model) paneContent(p paneID, w int) (string, []string) {
	if m.snap == nil {
		return "", nil
	}
	switch p {
	case pPackages:
		return m.packageRows(w)
	case pVulns:
		return m.vulnRows(w)
	default:
		return m.statusRows(w)
	}
}

func (m Model) packageRows(w int) (string, []string) {
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
	// Problems first: this pane is short and an outage shouldn't scroll away.
	status := slices.Clone(m.snap.Status)
	slices.SortStableFunc(status, func(a, b source.Status) int {
		return cmp.Compare(b2i(a.OK()), b2i(b.OK()))
	})
	for _, s := range status {
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

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
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
	lines = strings.Split(strings.Join(lines, "\n"), "\n") // a row may hold a line break
	if m.detail != "" {
		m.detailOff = min(m.detailOff, max(0, len(lines)-ch))
		lines = lines[m.detailOff:]
	}
	out := make([]string, 0, ch+2)
	out = append(out, "")
	for i := range ch {
		l := ""
		if i < len(lines) {
			l = highlight(clip(lines[i], cw), cw)
		}
		out = append(out, " "+l)
	}
	return strings.Join(append(out, ""), "\n")
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
		sTitle.Render("Help") + sDim.Render("   any key to close"),
		"",
		sBold.Render("Everywhere"),
		row("ctrl+p", "command palette: every action, every project"),
		row("1-7", "switch tab: Home, Projects, Agents, Deploys, GitHub, Health, Activity"),
		row("j/k, ↑/↓", "move"),
		row("g / G, pgup / pgdn", "top / bottom, page"),
		row("esc", "back"),
		row("r", "refresh now"),
		row("q", "quit"),
		"",
		sBold.Render("Agents"),
		row("n", "start an agent in its own worktree and branch"),
		row("a", "attach to an agent oryn started (ctrl+] comes back)"),
		row("x", "stop it; its worktree and branch stay"),
		row("enter", "details: what it's doing, and how to get back to it"),
		"",
		sBold.Render("Activity"),
		row("f", "filter: all, deploys, github, health"),
		row("e", "expand / fold vulnerabilities grouped by package"),
		"",
		sBold.Render("Home"),
		row("tab / shift+tab, h/l", "next / previous window"),
		row("enter", "open the selected row"),
		"",
		sBold.Render("Projects"),
		row("enter", "open the project"),
		row("a", "add a project directory (prefilled with where you launched oryn)"),
		row("d", "stop watching the selected project"),
		row("[  ]", "inside a project: previous / next tab (Overview, Agents, Deploys, GitHub, Health, Activity)"),
		row("h/l", "GitHub tab: Overview, PRs, Issues, Actions, Commits, People, Releases, Alerts"),
		row("enter / o", "GitHub tab: open the selected row / the repo in your browser"),
		row("p  D  g", "Overview tab: deploy · Deploys tab · GitHub tab"),
		"",
		sBold.Render("Deploys"),
		row("p", "deploy the linked project (asks first)"),
		row("d / X", "delete a deployment / the whole project (asks first)"),
		row("u", "unlink the folder in oryn only"),
		row("n (link picker)", "create a new project on the platform"),
		row("A", "connect Vercel / Netlify / Cloudflare accounts"),
		"",
		sBold.Render("Health"),
		row("tab / shift+tab, h/l", "next / previous pane"),
		row("enter", "open package"),
		row("/ or :", "fuzzy-find a package, e.g. `lod`, `chi`; tab completes, enter opens"),
		row("p  v  s  t", "search shortcuts: packages · vulns · status · activity"),
		"",
		sDim.Render("Config: ~/.config/oryn/config.toml · data: ~/.local/share/oryn/oryn.db"),
	}
}
