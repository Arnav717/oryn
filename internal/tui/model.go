// Package tui is the Bubble Tea terminal UI.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
)

type paneID int

const (
	pPortfolio paneID = iota
	pVulns
	pStatus
	pTape
	numPanes
)

var paneTitles = [numPanes]string{"PORTFOLIO", "VULNS", "UPSTREAM", "TAPE"}

// pkgInfo aggregates one package across versions and projects.
type pkgInfo struct {
	Eco, Name string
	Versions  []string
	Projects  []string
	Direct    bool
	DevOnly   bool
	Latest    string
	Vulns     []source.Vuln
}

func (p *pkgInfo) key() string { return source.PkgKey(p.Eco, p.Name) }

func (p *pkgInfo) maxSeverity() int {
	r := -1
	for _, v := range p.Vulns {
		r = max(r, source.SeverityRank(v.Severity))
	}
	return r
}

func (p *pkgInfo) outdated() bool {
	return slices.ContainsFunc(p.Versions, func(v string) bool { return source.Outdated(v, p.Latest) })
}

// screen is the top-level view. Home is the launcher; new sections plug in
// as extra homeItem kinds.
type screen int

const (
	screenHome screen = iota
	screenProject
	screenChanges
	screenDeploys
	screenGitHub
)

type snapMsg struct{ snap *app.Snapshot }
type clockMsg time.Time
type autoRefreshMsg struct{}

type Model struct {
	cfg *config.Config
	st  *store.Store

	snap     *app.Snapshot
	pkgs     map[string]*pkgInfo
	pkgOrder []string // portfolio rows: direct deps + vulnerable transitive ones
	vulns    []source.Vuln
	events   []store.Event // tape rows, limited to scope

	screen screen
	scope  string // project path shown on the project screen; "" = all projects

	stats map[string]projStat // by project path
	total projStat

	chCursor, chOffset int // changes screen
	chExpand           bool
	dpCursor, dpOffset int // deploys screen

	ghCursor int
	ghOffset int

	tab  projTab
	rsub subView
	rcur [numSubs]int
	roff [numSubs]int

	since       time.Time     // previous visit; zero on first run
	sinceDB     []store.Event // events since then, straight from the store
	sinceLoaded bool
	cwd         string
	homeCursor  int
	adding      bool
	pathInput   textinput.Model
	confirmRm   bool
	pendingScan bool // a project changed while a refresh was running

	loading bool
	spin    spinner.Model
	now     time.Time
	flash   string

	w, h   int
	focus  paneID
	cursor [numPanes]int
	offset [numPanes]int

	cmdMode bool
	input   textinput.Model

	detail    string // PkgKey of the open DES page, "" when closed
	detailOff int
	showHelp  bool
}

func New(cfg *config.Config, st *store.Store) Model {
	in := textinput.New()
	in.Prompt = sAccent.Render("> ")
	in.Placeholder = "type a package · v vulns · s status · t tape · h home · r refresh · ? help"

	path := textinput.New()
	path.Prompt = sAccent.Render("add project > ")
	path.Placeholder = "path to a project directory"

	cwd, _ := os.Getwd()
	m := Model{
		cfg:       cfg,
		st:        st,
		pkgs:      map[string]*pkgInfo{},
		stats:     map[string]projStat{},
		loading:   true,
		spin:      spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(sAccent)),
		now:       time.Now(),
		input:     in,
		pathInput: path,
		cwd:       cwd,
	}
	if st != nil {
		m.since, _ = st.TouchLastSeen(m.now)
	}
	m.homeCursor = m.cwdItem()
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), clock(), m.spin.Tick, m.autoRefresh())
}

func (m Model) refresh() tea.Cmd {
	// Snapshot the config now: the project list can change on the UI goroutine
	// while the refresh runs.
	cfg := *m.cfg
	cfg.Projects = slices.Clone(m.cfg.Projects)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return snapMsg{app.Refresh(ctx, &cfg, m.st)}
	}
}

func clock() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) })
}

func (m Model) autoRefresh() tea.Cmd {
	return tea.Tick(m.cfg.RefreshEvery, func(time.Time) tea.Msg { return autoRefreshMsg{} })
}

func (m *Model) startRefresh() tea.Cmd {
	if m.loading {
		m.pendingScan = true // run again once the current refresh lands
		return nil
	}
	m.pendingScan = false
	m.loading = true
	return tea.Batch(m.refresh(), m.spin.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clampAll()
		return m, nil

	case snapMsg:
		m.loading = false
		m.snap = msg.snap
		m.index()
		m.loadSince()
		m.clampAll()
		m.ghClamp()
		m.repoClamp()
		m.flash = "updated " + m.snap.At.Format("15:04:05")
		if n := len(m.snap.Warnings); n > 0 {
			m.flash += fmt.Sprintf(" · %d warning(s): %s", n, m.snap.Warnings[0])
		}
		if m.pendingScan {
			return m, m.startRefresh()
		}
		return m, nil

	case clockMsg:
		m.now = time.Time(msg)
		return m, clock()

	case autoRefreshMsg:
		return m, tea.Batch(m.startRefresh(), m.autoRefresh())

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if m.cmdMode {
		switch k.String() {
		case "esc":
			m.cmdMode = false
			m.input.Blur()
			return m, nil
		case "enter":
			m.cmdMode = false
			m.input.Blur()
			return m.runCommand(m.input.Value())
		case "tab":
			m.complete()
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(k)
		return m, cmd
	}

	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	switch m.screen {
	case screenHome:
		return m.handleHomeKey(k)
	case screenGitHub:
		return m.handleGHKey(k)
	case screenChanges:
		return m.handleChangesKey(k)
	case screenDeploys:
		return m.handleDeployKey(k)
	}

	if m.screen == screenProject && m.tab == tabGitHub {
		return m.handleRepoKey(k)
	}

	if m.detail != "" {
		switch k.String() {
		case "esc", "q", "backspace", "h", "left":
			m.detail = ""
		case "j", "down":
			m.detailOff++
		case "k", "up":
			m.detailOff = max(0, m.detailOff-1)
		case ":", "/":
			return m.openCommand()
		}
		return m, nil
	}

	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		m.screen = screenHome
	case "]":
		m.switchTab(1)
	case "[":
		m.switchTab(-1)
	case ":", "/":
		return m.openCommand()
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "tab", "l", "right":
		m.focus = (m.focus + 1) % numPanes
	case "shift+tab", "h", "left":
		m.focus = (m.focus + numPanes - 1) % numPanes
	case "1", "2", "3", "4":
		m.focus = paneID(k.String()[0] - '1')
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "pgdown", "ctrl+d":
		m.move(m.visible(m.focus))
	case "pgup", "ctrl+u":
		m.move(-m.visible(m.focus))
	case "g", "home":
		m.move(-1 << 30)
	case "G", "end":
		m.move(1 << 30)
	case "enter":
		if key := m.selectedPkg(); key != "" {
			m.openDetail(key)
		}
	}
	return m, nil
}

func (m Model) openCommand() (tea.Model, tea.Cmd) {
	m.cmdMode = true
	m.input.SetValue("")
	return m, m.input.Focus()
}

func (m *Model) openDetail(key string) {
	m.detail = key
	m.detailOff = 0
}

// index rebuilds package aggregates from the snapshot.
func (m *Model) index() {
	m.pkgs = map[string]*pkgInfo{}
	m.computeStats()
	for _, d := range m.snap.Deps {
		if m.scope != "" && d.Project != m.scope {
			continue
		}
		k := source.PkgKey(d.Ecosystem, d.Name)
		p, ok := m.pkgs[k]
		if !ok {
			p = &pkgInfo{Eco: d.Ecosystem, Name: d.Name, DevOnly: true, Latest: m.snap.Latest[k]}
			m.pkgs[k] = p
		}
		if d.Version != "" && !slices.Contains(p.Versions, d.Version) {
			p.Versions = append(p.Versions, d.Version)
		}
		if proj := filepath.Base(d.Project); !slices.Contains(p.Projects, proj) {
			p.Projects = append(p.Projects, proj)
		}
		p.Direct = p.Direct || d.Direct
		p.DevOnly = p.DevOnly && d.Dev
	}

	m.vulns = m.snap.VulnsFor(m.scope)
	for _, v := range m.vulns {
		if p := m.pkgs[source.PkgKey(v.Ecosystem, v.Package)]; p != nil {
			p.Vulns = append(p.Vulns, v)
		}
	}

	m.pkgOrder = m.pkgOrder[:0]
	for k, p := range m.pkgs {
		if p.Direct || len(p.Vulns) > 0 {
			m.pkgOrder = append(m.pkgOrder, k)
		}
	}
	sort.Slice(m.pkgOrder, func(i, j int) bool {
		a, b := m.pkgs[m.pkgOrder[i]], m.pkgs[m.pkgOrder[j]]
		if sa, sb := a.maxSeverity(), b.maxSeverity(); sa != sb {
			return sa > sb
		}
		if len(a.Vulns) != len(b.Vulns) {
			return len(a.Vulns) > len(b.Vulns)
		}
		if oa, ob := a.outdated(), b.outdated(); oa != ob {
			return oa
		}
		return a.Name < b.Name
	})

	m.events = m.events[:0]
	for _, e := range m.snap.Events {
		if _, ok := m.pkgs[e.Entity]; ok || e.Kind == "status" || m.scope == "" {
			m.events = append(m.events, e)
		}
	}
}

func (m *Model) rowCount(p paneID) int {
	if m.snap == nil {
		return 0
	}
	switch p {
	case pPortfolio:
		return len(m.pkgOrder)
	case pVulns:
		return len(m.vulns)
	case pStatus:
		return len(m.snap.Status)
	case pTape:
		return len(m.events)
	}
	return 0
}

func (m *Model) move(delta int) {
	p := m.focus
	m.cursor[p] += delta
	m.clamp(p)
}

func (m *Model) clamp(p paneID) {
	n := m.rowCount(p)
	m.cursor[p] = max(0, min(m.cursor[p], n-1))
	vis := max(1, m.visible(p))
	if m.cursor[p] < m.offset[p] {
		m.offset[p] = m.cursor[p]
	}
	if m.cursor[p] >= m.offset[p]+vis {
		m.offset[p] = m.cursor[p] - vis + 1
	}
	m.offset[p] = max(0, min(m.offset[p], n-vis))
}

func (m *Model) clampAll() {
	for p := range numPanes {
		m.clamp(p)
	}
}

func (m *Model) selectedPkg() string {
	if m.snap == nil || m.rowCount(m.focus) == 0 {
		return ""
	}
	i := m.cursor[m.focus]
	switch m.focus {
	case pPortfolio:
		return m.pkgOrder[i]
	case pVulns:
		return source.PkgKey(m.vulns[i].Ecosystem, m.vulns[i].Package)
	case pTape:
		e := m.events[i]
		if _, ok := m.pkgs[e.Entity]; ok {
			return e.Entity
		}
	}
	return ""
}

// findPkg resolves a user query to a PkgKey: exact name first, then substring.
func (m *Model) findPkg(q string) string {
	q = strings.ToLower(q)
	for k, p := range m.pkgs {
		if strings.ToLower(p.Name) == q {
			return k
		}
	}
	for _, k := range m.pkgOrder {
		if strings.Contains(strings.ToLower(m.pkgs[k].Name), q) {
			return k
		}
	}
	for k, p := range m.pkgs {
		if strings.Contains(strings.ToLower(p.Name), q) {
			return k
		}
	}
	return ""
}
