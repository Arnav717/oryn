package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/detect"
	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
)

// projStat summarises one project (or all of them) for the home screen.
type projStat struct {
	Deps, Direct      int
	Vulns, Crit, High int
	MaxSev            int // severity rank of the worst vuln, -1 when clean
	Outdated          int
	Missing           bool // the project directory no longer exists
}

func (m *Model) computeStats() {
	m.stats = map[string]projStat{}
	by := map[string][]detect.Dep{}
	for _, d := range m.snap.Deps {
		by[d.Project] = append(by[d.Project], d)
	}
	for _, p := range m.cfg.Projects {
		st := m.statsFor(by[p])
		_, err := os.Stat(p)
		st.Missing = err != nil
		m.stats[p] = st
	}
	m.total = m.statsFor(m.snap.Deps)
}

func (m *Model) statsFor(deps []detect.Dep) projStat {
	st := projStat{MaxSev: -1, Deps: len(deps)}
	seen := map[string]bool{}
	outdated := map[string]bool{}
	for _, d := range deps {
		if d.Direct {
			st.Direct++
		}
		if k := source.DepKey(d); !seen[k] {
			seen[k] = true
			for _, v := range m.snap.Vulns[k] {
				st.Vulns++
				switch v.Severity {
				case "CRITICAL":
					st.Crit++
				case "HIGH":
					st.High++
				}
				st.MaxSev = max(st.MaxSev, source.SeverityRank(v.Severity))
			}
		}
		pk := source.PkgKey(d.Ecosystem, d.Name)
		if d.Direct && source.Outdated(d.Version, m.snap.Latest[pk]) {
			outdated[pk] = true
		}
	}
	st.Outdated = len(outdated)
	return st
}

type itemKind int

const (
	itemAll itemKind = iota
	itemProject
	itemGitHub
	itemAddCwd
	itemAddPath
)

type homeItem struct {
	kind itemKind
	path string
}

// openable items lead to a dashboard; the rest are actions.
func (it homeItem) openable() bool {
	return it.kind == itemAll || it.kind == itemProject || it.kind == itemGitHub
}

// homeItems is the launcher menu. Future sections (GitHub, costs, ...) become
// new itemKinds here.
func (m *Model) homeItems() []homeItem {
	var items []homeItem
	if len(m.cfg.Projects) > 1 {
		items = append(items, homeItem{kind: itemAll})
	}
	for _, p := range m.cfg.Projects {
		items = append(items, homeItem{kind: itemProject, path: p})
	}
	if m.cfg.GitHubEnabled() {
		items = append(items, homeItem{kind: itemGitHub})
	}
	if home, _ := os.UserHomeDir(); m.cwd != "" && m.cwd != home && m.watchedRoot(m.cwd) == "" {
		items = append(items, homeItem{kind: itemAddCwd, path: m.cwd})
	}
	return append(items, homeItem{kind: itemAddPath})
}

// watchedRoot returns the watched project containing dir, preferring the
// deepest match, or "".
func (m *Model) watchedRoot(dir string) string {
	best := ""
	for _, p := range m.cfg.Projects {
		if (dir == p || strings.HasPrefix(dir, p+string(filepath.Separator))) && len(p) > len(best) {
			best = p
		}
	}
	return best
}

// cwdItem is the menu row to preselect: the project you launched oryn in.
func (m *Model) cwdItem() int {
	if root := m.watchedRoot(m.cwd); root != "" {
		for i, it := range m.homeItems() {
			if it.kind == itemProject && it.path == root {
				return i
			}
		}
	}
	return 0
}

func (m *Model) clampHome() {
	m.homeCursor = max(0, min(m.homeCursor, len(m.homeItems())-1))
}

func (m *Model) openScope(scope string) {
	m.screen = screenProject
	m.scope = scope
	m.focus = pPortfolio
	m.cursor = [numPanes]int{}
	m.offset = [numPanes]int{}
	m.detail = ""
	m.tab, m.rsub = tabDeps, svOverview
	m.rcur, m.roff = [numSubs]int{}, [numSubs]int{}
	if m.snap != nil {
		m.index()
		m.clampAll()
	}
}

func (m Model) handleHomeKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.adding {
		switch k.String() {
		case "esc":
			m.adding = false
			m.pathInput.Blur()
			return m, nil
		case "enter":
			m.adding = false
			m.pathInput.Blur()
			return m.addProject(m.pathInput.Value())
		}
		var cmd tea.Cmd
		m.pathInput, cmd = m.pathInput.Update(k)
		return m, cmd
	}

	items := m.homeItems()
	m.clampHome()

	if m.confirmRm {
		m.confirmRm = false
		if k.String() == "y" && items[m.homeCursor].kind == itemProject {
			return m.removeProject(items[m.homeCursor].path)
		}
		return m, nil
	}

	switch s := k.String(); s {
	case "q", "esc":
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "j", "down":
		m.homeCursor = min(m.homeCursor+1, len(items)-1)
	case "k", "up":
		m.homeCursor = max(m.homeCursor-1, 0)
	case "g", "home":
		m.homeCursor = 0
	case "G", "end":
		m.homeCursor = len(items) - 1
	case "enter", "l", "right":
		return m.activate(items[m.homeCursor])
	case "A":
		return m.openAccounts("")
	case "D":
		m.screen = screenDeploys
		m.dpCursor, m.dpOffset = 0, 0
		m.dpClamp()
	case "c":
		m.screen = screenChanges
		m.chCursor, m.chOffset, m.chExpand = 0, 0, false
		m.chClamp()
	case "a":
		return m.startAdd(m.cwd)
	case "d", "x":
		if items[m.homeCursor].kind == itemProject {
			m.confirmRm = true
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(s[0] - '1'); i < len(items) && items[i].openable() {
			m.homeCursor = i
			return m.activate(items[i])
		}
	}
	return m, nil
}

func (m Model) activate(it homeItem) (tea.Model, tea.Cmd) {
	switch it.kind {
	case itemAll:
		m.openScope("")
	case itemProject:
		m.openScope(it.path)
	case itemGitHub:
		m.screen = screenGitHub
		m.ghCursor, m.ghOffset = 0, 0
		m.ghClamp()
	case itemAddCwd:
		return m.addProject(it.path)
	case itemAddPath:
		return m.startAdd(m.cwd)
	}
	return m, nil
}

func (m Model) startAdd(prefill string) (tea.Model, tea.Cmd) {
	if m.cfg.Ephemeral {
		m.flash = "projects were set with -p and can't be edited here"
		return m, nil
	}
	m.adding = true
	m.pathInput.SetValue(prefill)
	m.pathInput.CursorEnd()
	return m, m.pathInput.Focus()
}

func (m Model) addProject(raw string) (tea.Model, tea.Cmd) {
	if m.cfg.Ephemeral {
		m.flash = "projects were set with -p and can't be edited here"
		return m, nil
	}
	abs, err := filepath.Abs(config.ExpandHome(strings.TrimSpace(raw)))
	if err != nil {
		m.flash = err.Error()
		return m, nil
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		m.flash = config.ShortHome(abs) + " is not a directory"
		return m, nil
	}
	if slices.Contains(m.cfg.Projects, abs) {
		m.flash = "already watching " + config.ShortHome(abs)
		return m, nil
	}
	m.cfg.Projects = append(m.cfg.Projects, abs)
	m.flash = "watching " + config.ShortHome(abs)
	if err := m.cfg.Save(); err != nil {
		m.flash = "added, but could not save config: " + err.Error()
	}
	m.homeCursor = slices.Index(m.cfg.Projects, abs)
	if len(m.cfg.Projects) > 1 {
		m.homeCursor++ // "All projects" row now leads the list
	}
	return m, m.startRefresh()
}

func (m Model) removeProject(path string) (tea.Model, tea.Cmd) {
	if m.cfg.Ephemeral {
		m.flash = "projects were set with -p and can't be edited here"
		return m, nil
	}
	i := slices.Index(m.cfg.Projects, path)
	if i < 0 {
		return m, nil
	}
	m.cfg.Projects = slices.Delete(m.cfg.Projects, i, i+1)
	m.flash = "stopped watching " + config.ShortHome(path)
	if err := m.cfg.Save(); err != nil {
		m.flash = "removed, but could not save config: " + err.Error()
	}
	if m.snap != nil { // drop its deps now instead of waiting for the refresh
		m.snap.Deps = slices.DeleteFunc(slices.Clone(m.snap.Deps), func(d detect.Dep) bool { return d.Project == path })
		m.index()
		m.clampAll()
	}
	m.clampHome()
	return m, m.startRefresh()
}

// homeLines renders the launcher body.
func (m Model) homeLines() []string {
	cw := m.w - 2
	items := m.homeItems()
	n := len(m.cfg.Projects)
	loaded := m.snap != nil

	const nameW, depsW, vulnW, outW, ghW = 20, 7, 20, 10, 18
	pathW := max(8, cw-6-nameW-depsW-vulnW-outW-ghW)

	lines := []string{sTitle.Render("PROJECTS") + sDim.Render(fmt.Sprintf("  %d watched", n)), ""}
	if n == 0 {
		lines = append(lines, sDim.Render("  Nothing watched yet. Pick a directory below to start."), "")
	} else {
		lines = append(lines, sDim.Render("      "+pad("PROJECT", nameW)+pad("PATH", pathW)+pad("DEPS", depsW)+pad("VULNS", vulnW)+pad("OUTDATED", outW)+"GITHUB"))
	}

	addGap, sawIntegrations := true, false
	for i, it := range items {
		marker, num := "  ", "    "
		if i == m.homeCursor {
			marker = sAccent.Render("▸ ")
		}
		if it.openable() && i < 9 {
			num = sDim.Render(fmt.Sprintf("%d   ", i+1))
		}

		if it.kind == itemGitHub {
			if !sawIntegrations {
				sawIntegrations = true
				lines = append(lines, "", sTitle.Render("INTEGRATIONS"), "")
			}
			name := "GitHub"
			if i == m.homeCursor {
				name = sBold.Render(name)
			}
			lines = append(lines, marker+num+pad(name, nameW)+m.ghHomeSummary())
			continue
		}

		if !it.openable() {
			if addGap && n > 0 {
				lines = append(lines, "")
			}
			addGap = false
			label, hint := "Add a project…", sDim.Render("a")
			if it.kind == itemAddCwd {
				label, hint = "Add current directory", sDim.Render(config.ShortHome(it.path))
			}
			lines = append(lines, marker+sAccent.Render("+   ")+pad(label, nameW+2)+hint)
			continue
		}

		name, path, st := "All projects", sDim.Render(fmt.Sprintf("%d projects", n)), m.total
		if it.kind == itemProject {
			name, path, st = filepath.Base(it.path), sDim.Render(config.ShortHome(it.path)), m.stats[it.path]
		}
		if i == m.homeCursor {
			name = sBold.Render(name)
		}
		deps, vulns, outdated := sDim.Render("…"), sDim.Render("…"), sDim.Render("…")
		// A project with no data while a refresh runs is still being scanned.
		if loaded && !(m.loading && st.Deps == 0) {
			deps, vulns, outdated = fmt.Sprint(st.Deps), vulnCell(st), outdatedCell(st)
			if it.kind == itemProject && st.Missing {
				vulns, outdated = sBad.Render("directory missing"), ""
			}
		}
		ghCell := ""
		if it.kind == itemProject && loaded {
			ghCell = m.repoCell(it.path)
		}
		lines = append(lines, marker+num+pad(name, nameW)+pad(path, pathW)+pad(deps, depsW)+pad(vulns, vulnW)+pad(outdated, outW)+ghCell)
	}

	lines = append(lines, m.sinceLines()...)

	lines = append(lines, m.deployLines()...)

	lines = append(lines, "", sTitle.Render("UPSTREAM"))
	lines = append(lines, m.upstreamLines()...)

	lines = append(lines, "", sTitle.Render("RECENT"))
	if !loaded || len(m.snap.Events) == 0 {
		lines = append(lines, sDim.Render("  nothing yet"))
	} else {
		for _, e := range m.snap.Events[:min(6, len(m.snap.Events))] {
			lines = append(lines, "  "+eventLine(e))
		}
	}
	return lines
}

func vulnCell(st projStat) string {
	switch {
	case st.Deps == 0:
		return sDim.Render("no lockfiles found")
	case st.Vulns == 0:
		return sOK.Render("✓ clean")
	}
	detail := ""
	switch {
	case st.Crit > 0:
		detail = fmt.Sprintf(" %d crit", st.Crit)
	case st.High > 0:
		detail = fmt.Sprintf(" %d high", st.High)
	}
	return rankStyle(st.MaxSev).Render(fmt.Sprintf("▲ %d", st.Vulns)) + sDim.Render(detail)
}

func outdatedCell(st projStat) string {
	switch {
	case st.Deps == 0:
		return ""
	case st.Outdated == 0:
		return sOK.Render("✓")
	}
	return sWarn.Render(fmt.Sprint(st.Outdated))
}

// upstreamLines lists services with problems, or one line when all is well.
func (m Model) upstreamLines() []string {
	if m.snap == nil {
		return []string{sDim.Render("  checking…")}
	}
	var lines []string
	ok := 0
	for _, s := range m.snap.Status {
		switch {
		case s.OK():
			ok++
		case s.Err != nil:
			lines = append(lines, "  "+sDim.Render("? "+pad(s.Service, 14)+"unreachable"))
		case s.Indicator == "major" || s.Indicator == "critical":
			lines = append(lines, "  "+sBad.Render("● "+pad(s.Service, 14)+s.Description))
		default:
			lines = append(lines, "  "+sWarn.Render("● "+pad(s.Service, 14)+s.Description))
		}
	}
	via := m.stackVia()
	if len(lines) == 0 {
		lines = []string{"  " + sOK.Render(fmt.Sprintf("● all %d services operational", ok))}
		if via != "" {
			lines = append(lines, "  "+sDim.Render("  from your stack: "+via))
		}
		return lines
	}
	if ok > 0 {
		lines = append(lines, "  "+sDim.Render(fmt.Sprintf("%d others operational", ok)))
	}
	return lines
}

// upstreamPart is the one-phrase upstream summary for the header.
func (m Model) upstreamPart() string {
	down := 0
	for _, s := range m.snap.Status {
		if !s.OK() {
			down++
		}
	}
	if down > 0 {
		return sBad.Render(fmt.Sprintf("%d upstream issue(s)", down))
	}
	return sOK.Render("upstream ok")
}

// sinceEvents lists tape events newer than the previous visit, most severe
// first. Repo baselines are bookkeeping, not news.
func (m Model) sinceEvents() []store.Event {
	if m.snap == nil || m.since.IsZero() {
		return nil
	}
	if m.sinceLoaded {
		return m.sinceDB
	}
	var out []store.Event
	seen := map[string]bool{}
	for _, e := range append(append([]store.Event(nil), m.snap.New...), m.snap.Events...) {
		if e.Kind == "repo" || seen[e.Key] || (!e.At.After(m.since) && !isNewKey(m.snap.New, e.Key)) {
			continue
		}
		seen[e.Key] = true
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity > out[j].Severity })
	return out
}

func isNewKey(evs []store.Event, key string) bool {
	return slices.ContainsFunc(evs, func(e store.Event) bool { return e.Key == key })
}

// sinceRow is one display line of the since section: a single event, or
// several vulns in the same package folded together.
type sinceRow struct {
	sev  int
	line string
	url  string
	evs  []store.Event // every event folded into this row
}

// groupSince folds vuln events for the same package@version into one row so a
// single bad package doesn't crowd out everything else. Order follows evs.
func groupSince(evs []store.Event) []sinceRow {
	var rows []sinceRow
	idx := map[string]int{}   // package@version -> index into rows
	count := map[string]int{} // package@version -> vulns folded
	first := map[string]store.Event{}
	for _, e := range evs {
		pkg := vulnPkg(e)
		if pkg == "" {
			rows = append(rows, sinceRow{e.Severity, eventLine(e), e.URL, []store.Event{e}})
			continue
		}
		count[pkg]++
		if count[pkg] == 1 {
			first[pkg] = e
			idx[pkg] = len(rows)
			rows = append(rows, sinceRow{e.Severity, eventLine(e), e.URL, []store.Event{e}})
			continue
		}
		rows[idx[pkg]].evs = append(rows[idx[pkg]].evs, e)
		f := first[pkg]
		rows[idx[pkg]].line = sDim.Render(f.At.Format("Jan02 15:04")) + " " + rankStyle(f.Severity).Render(pad("VULN", 7)) + " " +
			fmt.Sprintf("%d vulns in %s, worst %s", count[pkg], pkg, strings.ToLower(sevName(f.Severity)))
	}
	return rows
}

// vulnPkg extracts "package@version" from a vuln event key
// ("vuln:<id>:<ecosystem>:<package>@<version>"), or "" for other events.
func vulnPkg(e store.Event) string {
	if e.Kind != "vuln" {
		return ""
	}
	parts := strings.SplitN(e.Key, ":", 4)
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

func sevName(rank int) string {
	return []string{"unknown", "low", "medium", "high", "critical"}[min(max(rank, 0), 4)]
}

func (m Model) sinceLines() []string {
	evs := m.sinceEvents()
	if len(evs) == 0 {
		return nil
	}
	rows := groupSince(evs)
	head := sTitle.Render("SINCE YOU LAST LOOKED") + sDim.Render(fmt.Sprintf("  %s ago · %d new", shortAgo(m.now.Sub(m.since)), len(evs)))
	lines := []string{"", head}
	for _, r := range rows[:min(6, len(rows))] {
		lines = append(lines, "  "+r.line)
	}
	if len(rows) > 6 {
		lines = append(lines, sDim.Render(fmt.Sprintf("  …and %d more changes (press c)", len(rows)-6)))
	}
	return lines
}

func shortAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "moments"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// loadSince reads everything newer than the previous visit from the store,
// since the in-memory tape only holds the latest 200 events.
func (m *Model) loadSince() {
	m.sinceLoaded = false
	if m.st == nil || m.since.IsZero() {
		return
	}
	evs, err := m.st.Since(m.since)
	if err != nil {
		return
	}
	m.sinceDB, m.sinceLoaded = evs, true
}

// stackVia names services watched because of a lockfile dependency.
func (m Model) stackVia() string {
	var parts []string
	for _, s := range m.snap.Status {
		if s.Via != "" {
			parts = append(parts, s.Service+" ("+s.Via+")")
		}
	}
	return strings.Join(parts, ", ")
}
