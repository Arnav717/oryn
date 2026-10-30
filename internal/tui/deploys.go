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

func targetNote(t app.DeployTarget) string {
	switch {
	case !t.Queryable && deploy.ProviderFor(t.Link.Platform) != nil:
		return "detected via " + t.Link.Marker + ", not linked: run `" + t.Link.Platform + " link`"
	case !t.Queryable:
		return "detected via " + t.Link.Marker + " (not queryable yet)"
	case errors.Is(t.Err, deploy.ErrNoCredentials):
		return "no credentials: log in with the " + t.Link.Platform + " CLI or set its token"
	case t.Err != nil:
		return "error: " + t.Err.Error()
	}
	return ""
}

// deployLines is the compact DEPLOYS block on the home screen; nil when no
// project has any deployment config.
func (m Model) deployLines() []string {
	if m.snap == nil || len(m.snap.Deploys) == 0 {
		return nil
	}
	lines := []string{"", sTitle.Render("DEPLOYS")}
	for _, t := range m.snap.Deploys {
		name := pad(filepath.Base(t.Link.Project), 20)
		if note := targetNote(t); note != "" {
			lines = append(lines, "  "+sDim.Render("? "+name+pad(t.Link.Platform, 12)+note))
			continue
		}
		d, ok := t.Latest("production")
		if !ok && len(t.Deployments) > 0 {
			d, ok = t.Deployments[0], true
		}
		if !ok {
			lines = append(lines, "  "+sDim.Render("? "+name+pad(t.Link.Platform, 12)+"no deployments"))
			continue
		}
		lines = append(lines, "  "+deployDot(d.State)+" "+name+pad(t.Link.Platform, 12)+pad(d.State, 10)+
			sDim.Render(pad(d.Branch, 12)+pad(d.Commit, 9)+ago(d.Created, m.now)))
	}
	return lines
}

type deployRow struct {
	line string
	url  string
}

// deployRows flattens every target into a header plus one row per deployment.
func (m Model) deployRows() (lines []string, rows []deployRow, rowLine []int) {
	if m.snap == nil {
		return []string{sDim.Render("  loading…")}, nil, nil
	}
	if len(m.snap.Deploys) == 0 {
		return []string{sDim.Render("  No Vercel, Netlify or Cloudflare Pages config found in watched projects.")}, nil, nil
	}
	for _, t := range m.snap.Deploys {
		lines = append(lines, "", sBold.Render(filepath.Base(t.Link.Project))+" "+sDim.Render(t.Link.Platform+" "+t.Link.Ref))
		if note := targetNote(t); note != "" {
			lines = append(lines, "  "+sDim.Render(note))
			continue
		}
		for _, d := range t.Deployments {
			msg := strings.TrimSpace(d.Message)
			l := deployDot(d.State) + " " + pad(d.State, 9) + pad(d.Target, 11) + pad(d.Branch, 14) + pad(d.Commit, 9) +
				sDim.Render(pad(ago(d.Created, m.now), 10)) + msg
			rowLine = append(rowLine, len(lines))
			rows = append(rows, deployRow{l, d.URL})
			lines = append(lines, "  "+l)
		}
	}
	return lines, rows, rowLine
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
	case "esc", "h", "D", "backspace", "left":
		m.screen = screenHome
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
	case "enter", "o", "l", "right":
		if _, rows, _ := m.deployRows(); m.dpCursor >= 0 && m.dpCursor < len(rows) && rows[m.dpCursor].url != "" {
			if err := browser.Open(rows[m.dpCursor].url); err != nil {
				m.flash = err.Error()
			} else {
				m.flash = "opened deployment"
			}
		}
	}
	m.dpClamp()
	return m, nil
}

func (m Model) deploySummary() string {
	if m.snap == nil {
		return ""
	}
	failing, live := 0, 0
	for _, t := range m.snap.Deploys {
		if d, ok := t.Latest("production"); ok {
			live++
			if d.State == deploy.StateError {
				failing++
			}
		}
	}
	s := fmt.Sprintf("%d live", live)
	if failing > 0 {
		s += " · " + sBad.Render(fmt.Sprintf("%d failing", failing))
	}
	return s
}
