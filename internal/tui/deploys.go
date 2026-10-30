package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/browser"
	"github.com/Arnav717/oryn/internal/deploy"
)

func deployDot(state string) string {
	switch state {
	case deploy.StateReady:
		return sOK.Render("●")
	case deploy.StateError:
		return sBad.Render("●")
	case deploy.StateCanceled:
		return sDim.Render("●")
	}
	return sWarn.Render("◌") // building, queued
}

// ago is a compact relative time for list rows.
func ago(t time.Time, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	return shortAgo(now.Sub(t)) + " ago"
}

// actionHint says what pressing enter will do on a note row.
func actionHint(t app.DeployTarget) string {
	switch {
	case errors.Is(t.Err, deploy.ErrNoCredentials), t.Err == nil && deploy.Source(t.Link.Platform) == "" && deploy.ConnectorFor(t.Link.Platform) != nil && !t.Queryable:
		return "  [enter: connect account]"
	case !t.Queryable && deploy.ConnectorFor(t.Link.Platform) != nil:
		return "  [enter: pick project]"
	}
	return ""
}

func targetNote(t app.DeployTarget) string {
	switch {
	case !t.Queryable && deploy.ProviderFor(t.Link.Platform) != nil:
		return "detected via " + t.Link.Marker + ", not linked to a " + t.Link.Platform + " project"
	case !t.Queryable:
		return "detected via " + t.Link.Marker + " (not queryable yet)"
	case errors.Is(t.Err, deploy.ErrNoCredentials):
		return "no " + t.Link.Platform + " account connected"
	case t.Err != nil:
		return "error: " + t.Err.Error()
	}
	return ""
}

type deployRow struct {
	line   string
	url    string
	target int    // index into snap.Deploys
	note   bool   // a status note (not linked, no credentials) rather than a deployment
	add    string // platform offered for linking when the project has no target for it
	dep    *deploy.Deployment
}

// deployRows flattens every target into a header plus one row per deployment.
func (m Model) deployRows() (lines []string, rows []deployRow, rowLine []int) {
	if m.snap == nil {
		return []string{sDim.Render("  loading…")}, nil, nil
	}
	if len(m.snap.Deploys) == 0 && m.deployScope() == "" {
		return []string{
			sDim.Render("  No Vercel, Netlify or Cloudflare Pages config found in watched projects."),
			"",
			sDim.Render("  Open a project and press D to link it to a platform, or press A to connect an account."),
		}, nil, nil
	}
	scope := m.deployScope()
	shown := 0
	for ti, t := range m.snap.Deploys {
		if scope != "" && t.Link.Project != scope {
			continue
		}
		shown++
		lines = append(lines, "", sBold.Render(filepath.Base(t.Link.Project))+" "+sDim.Render(t.Link.Platform+" "+orDefault(t.Link.Name, t.Link.Ref)))
		if note := targetNote(t); note != "" {
			rowLine = append(rowLine, len(lines))
			rows = append(rows, deployRow{line: note, target: ti, note: true})
			lines = append(lines, "  "+sDim.Render(note+actionHint(t)))
			continue
		}
		for _, d := range t.Deployments {
			msg := strings.TrimSpace(d.Message)
			l := deployDot(d.State) + " " + pad(d.State, 9) + pad(d.Target, 11) + pad(d.Branch, 14) + pad(d.Commit, 9) +
				sDim.Render(pad(ago(d.Created, m.now), 10)) + msg
			rowLine = append(rowLine, len(lines))
			d := d
			rows = append(rows, deployRow{line: l, url: d.URL, target: ti, dep: &d})
			lines = append(lines, "  "+l)
		}
	}
	if scope != "" {
		have := map[string]bool{}
		for _, t := range m.snap.Deploys {
			if t.Link.Project == scope {
				have[t.Link.Platform] = true
			}
		}
		var offer []string
		for _, p := range deploy.Platforms {
			if !have[p] {
				offer = append(offer, p)
			}
		}
		if len(offer) > 0 {
			head := "Link this project to a platform"
			if shown == 0 {
				head = "No deploy config found in this project. Link it to a platform"
			}
			lines = append(lines, "", sDim.Render(head+" (enter):"))
			for _, p := range offer {
				state := "connect account first"
				if deploy.Source(p) != "" {
					state = "pick a project"
				}
				rowLine = append(rowLine, len(lines))
				rows = append(rows, deployRow{line: p, target: -1, add: p})
				lines = append(lines, "  "+sAccent.Render("+")+" "+pad(p, 12)+sDim.Render(state))
			}
		}
	}
	return lines, rows, rowLine
}

// deployScope is the project the deploys view is limited to: the open project
// on its Deploys tab, everything on the central screen.
func (m Model) deployScope() string {
	if m.screen == screenProject && m.tab == tabDeploys {
		return m.scope
	}
	return ""
}

// deployPlatforms names the platforms a project deploys to, for the tab label.
func (m Model) deployPlatforms(project string) string {
	if m.snap == nil {
		return ""
	}
	var names []string
	for _, t := range m.snap.Deploys {
		if t.Link.Project == project {
			names = append(names, t.Link.Platform)
		}
	}
	return strings.Join(names, ", ")
}

func (m *Model) dpClamp() {
	_, rows, rowLine := m.deployRows()
	m.dpCursor = max(0, min(m.dpCursor, len(rows)-1))
	if m.h == 0 || len(rowLine) == 0 {
		return
	}
	vis := max(1, m.h-m.chrome()-2)
	line := rowLine[m.dpCursor]
	if line < m.dpOffset {
		m.dpOffset = line
	}
	if line >= m.dpOffset+vis {
		m.dpOffset = line - vis + 1
	}
	m.dpOffset = max(0, m.dpOffset)
}

func (m Model) deployBody() []string {
	lines, rows, rowLine := m.deployRows()
	if len(rows) > 0 {
		i := rowLine[min(m.dpCursor, len(rows)-1)]
		lines[i] = sAccent.Render("▸") + lines[i][1:]
	}
	return lines[min(m.dpOffset, len(lines)):]
}

func (m Model) handleDeployKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.screen == screenProject {
			m.setTab(tabOverview)
		} else {
			m.openSection(secHome)
		}
	case "h", "backspace", "left", "D":
		if m.screen == screenDeploys {
			m.openSection(secHome)
		}
	case "]":
		if m.screen == screenProject {
			m.switchTab(1)
		}
	case "[":
		if m.screen == screenProject {
			m.switchTab(-1)
		}
	case ":", "/":
		return m.openCommand()
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "j", "down":
		m.dpCursor++
	case "k", "up":
		m.dpCursor--
	case "g", "home":
		m.dpCursor = 0
	case "G", "end":
		m.dpCursor = 1 << 30
	case "A":
		return m.openAccounts("")
	case "d":
		return m.askDeleteDeployment()
	case "X":
		return m.askDeleteProject()
	case "u":
		return m.unlink()
	case "p":
		if t, ok := m.deployTargetAtCursor(); ok {
			if !t.Link.Linked() {
				m.flash = "link a project first (enter), then press p to deploy"
				return m, nil
			}
			return m.askDeploy(t)
		}
		m.flash = "nothing selected to deploy"
	case "l":
		if t, ok := m.deployTargetAtCursor(); ok {
			return m.linkTarget(t)
		}
		m.flash = "nothing selected to link"
	case "enter", "o", "right":
		_, rows, _ := m.deployRows()
		if m.dpCursor < 0 || m.dpCursor >= len(rows) {
			break
		}
		r := rows[m.dpCursor]
		if r.add != "" {
			return m.linkTarget(app.DeployTarget{Link: deploy.Link{Platform: r.add, Project: m.scope}})
		}
		if r.note {
			return m.linkTarget(m.snap.Deploys[r.target])
		}
		if r.url != "" {
			if err := browser.Open(r.url); err != nil {
				m.flash = err.Error()
			} else {
				m.flash = "opened deployment"
			}
		}
	}
	m.dpClamp()
	return m, nil
}

// deployCounts is how many projects have a production deploy, and how many of
// those failed.
func (m Model) deployCounts() (failing, live int) {
	if m.snap == nil {
		return 0, 0
	}
	for _, t := range m.snap.Deploys {
		if d, ok := t.Latest("production"); ok {
			live++
			if d.State == deploy.StateError {
				failing++
			}
		}
	}
	return failing, live
}

func (m Model) deploySummary() string {
	if m.snap == nil {
		return ""
	}
	failing, live := m.deployCounts()
	s := fmt.Sprintf("%d live", live)
	if failing > 0 {
		s += " · " + sBad.Render(fmt.Sprintf("%d failing", failing))
	}
	return s
}

func (m Model) deployTargetAtCursor() (app.DeployTarget, bool) {
	_, rows, _ := m.deployRows()
	if m.dpCursor < 0 || m.dpCursor >= len(rows) {
		return app.DeployTarget{}, false
	}
	if r := rows[m.dpCursor]; r.add != "" {
		return app.DeployTarget{Link: deploy.Link{Platform: r.add, Project: m.scope}}, true
	}
	return m.snap.Deploys[rows[m.dpCursor].target], true
}

// pickLinesScrolled keeps the picker cursor in view for long project lists.
func (m Model) pickLinesScrolled() []string {
	lines := m.pickLines()
	vis := max(1, m.h-m.chrome()-2)
	cur := m.pickCursor + 2
	if cur >= vis && !m.pickLoading && m.pickErr == nil {
		lines = append(lines[:2:2], lines[min(len(lines), cur-vis+3):]...)
	}
	return lines
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
