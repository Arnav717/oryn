package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/browser"
	"github.com/Arnav717/oryn/internal/deploy"
)

type connectMsg struct {
	platform, who string
	err           error
}

type projectsMsg struct {
	platform string
	items    []deploy.ProjectRef
	err      error
}

func connectCmd(platform, token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		who, err := deploy.Connect(ctx, platform, token)
		return connectMsg{platform, who, err}
	}
}

func projectsCmd(platform string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		c := deploy.ConnectorFor(platform)
		if c == nil {
			return projectsMsg{platform: platform, err: fmt.Errorf("unknown platform")}
		}
		items, err := c.Projects(ctx)
		return projectsMsg{platform, items, err}
	}
}

func newTokenInput() textinput.Model {
	in := textinput.New()
	in.Prompt = sAccent.Render("token > ")
	in.EchoMode = textinput.EchoPassword
	in.EchoCharacter = '•'
	in.CharLimit = 512
	return in
}

// openAccounts shows the accounts screen, with platform's row selected.
func (m Model) openAccounts(platform string) (tea.Model, tea.Cmd) {
	m.screen = screenAccounts
	m.acctCursor = 0
	for i, p := range deploy.Platforms {
		if p == platform {
			m.acctCursor = i
		}
	}
	return m, nil
}

func (m Model) acctLines() []string {
	lines := []string{sTitle.Render("ACCOUNTS") + sDim.Render("  tokens are verified with the platform before they are saved"), ""}
	for i, p := range deploy.Platforms {
		marker := "  "
		if i == m.acctCursor {
			marker = sAccent.Render("▸ ")
		}
		state := sDim.Render("not connected")
		dot := sDim.Render("○")
		switch src := deploy.Source(p); src {
		case "":
		case "env":
			dot, state = sOK.Render("●"), sOK.Render("connected")+sDim.Render(" via environment variable")
		case "cli":
			dot, state = sOK.Render("●"), sOK.Render("connected")+sDim.Render(" via the "+p+" CLI login")
		default:
			dot, state = sOK.Render("●"), sOK.Render("connected")+sDim.Render(" (token saved by oryn)")
		}
		lines = append(lines, marker+dot+" "+pad(p, 12)+state)
	}
	lines = append(lines, "",
		sDim.Render("  enter  paste an API token     b  open the token page in your browser"),
		sDim.Render("  x      forget a token oryn saved"),
		"",
		sDim.Render("  Tokens saved by oryn live in "+filepath.Join("~/.config/oryn", "credentials.toml")+" (owner-only, 0600)."))
	if p := deploy.Platforms[m.acctCursor]; p == "cloudflare" {
		lines = append(lines, "", sDim.Render("  Cloudflare: create a token with Account Settings: Read and Cloudflare Pages: Read."))
	}
	return lines
}

func (m Model) handleAccountsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tokenEntry {
		switch k.String() {
		case "esc":
			m.tokenEntry = false
			m.tokenInput.Blur()
			m.tokenInput.SetValue("")
			return m, nil
		case "enter":
			tok := strings.TrimSpace(m.tokenInput.Value())
			m.tokenEntry = false
			m.tokenInput.Blur()
			m.tokenInput.SetValue("")
			if tok == "" {
				return m, nil
			}
			m.flash = "checking token…"
			return m, connectCmd(deploy.Platforms[m.acctCursor], tok)
		}
		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(k)
		return m, cmd
	}
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "h", "A", "backspace", "left":
		m.screen = screenHome
	case "?":
		m.showHelp = true
	case "j", "down":
		m.acctCursor = min(m.acctCursor+1, len(deploy.Platforms)-1)
	case "k", "up":
		m.acctCursor = max(m.acctCursor-1, 0)
	case "enter", "t", "right":
		m.tokenEntry = true
		m.tokenInput = newTokenInput()
		m.tokenInput.Focus()
	case "b":
		p := deploy.Platforms[m.acctCursor]
		if err := browser.Open(deploy.TokenPage[p]); err != nil {
			m.flash = err.Error()
		} else {
			m.flash = "opened " + p + " token page"
		}
	case "x":
		p := deploy.Platforms[m.acctCursor]
		if deploy.Source(p) != "oryn" {
			m.flash = "nothing saved by oryn for " + p
			break
		}
		if err := deploy.Disconnect(p); err != nil {
			m.flash = err.Error()
		} else {
			m.flash = "forgot " + p + " token"
			return m, m.startRefresh()
		}
	}
	return m, nil
}

// linkTarget does the right next step for a deploy target that isn't
// queryable: connect the account first, then pick the project.
func (m Model) linkTarget(t app.DeployTarget) (tea.Model, tea.Cmd) {
	if deploy.ConnectorFor(t.Link.Platform) == nil {
		m.flash = "nothing to link for " + t.Link.Platform
		return m, nil
	}
	if deploy.Source(t.Link.Platform) == "" {
		return m.openAccounts(t.Link.Platform)
	}
	if t.Queryable && t.Err == nil {
		m.flash = "already linked to " + t.Link.Ref + " (press l again on an unlinked target)"
		return m, nil
	}
	m.pickTarget = t
	m.pickItems, m.pickErr, m.pickCursor, m.pickLoading = nil, nil, 0, true
	m.screen = screenPicker
	return m, projectsCmd(t.Link.Platform)
}

func (m Model) pickLines() []string {
	head := sTitle.Render("LINK PROJECT") + sDim.Render(fmt.Sprintf("  %s ← %s project", filepath.Base(m.pickTarget.Link.Project), m.pickTarget.Link.Platform))
	switch {
	case m.pickLoading:
		return []string{head, "", sDim.Render("  loading projects…")}
	case m.pickErr != nil:
		return []string{head, "", sBad.Render("  " + m.pickErr.Error())}
	case len(m.pickItems) == 0:
		return []string{head, "", sDim.Render("  This account has no projects on " + m.pickTarget.Link.Platform + ".")}
	}
	lines := []string{head, ""}
	for i, it := range m.pickItems {
		marker := "  "
		if i == m.pickCursor {
			marker = sAccent.Render("▸ ")
		}
		lines = append(lines, marker+it.Name)
	}
	return lines
}

func (m Model) handlePickerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "h", "backspace", "left":
		m.screen = screenDeploys
	case "j", "down":
		m.pickCursor = min(m.pickCursor+1, max(0, len(m.pickItems)-1))
	case "k", "up":
		m.pickCursor = max(m.pickCursor-1, 0)
	case "enter":
		if m.pickLoading || m.pickCursor >= len(m.pickItems) {
			break
		}
		it, t := m.pickItems[m.pickCursor], m.pickTarget
		m.cfg.SetDeployLink(t.Link.Project, t.Link.Platform, it.Ref, it.Team)
		m.screen = screenDeploys
		if !m.cfg.Ephemeral {
			if err := m.cfg.Save(); err != nil {
				m.flash = "linked, but couldn't save config: " + err.Error()
				return m, m.startRefresh()
			}
		}
		m.flash = "linked " + filepath.Base(t.Link.Project) + " to " + it.Name
		return m, m.startRefresh()
	}
	return m, nil
}
