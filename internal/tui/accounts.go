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
		lines = append(lines, "", sDim.Render("  Cloudflare: create a token with Account Settings: Read and Cloudflare Pages: Edit (Read is enough to view deployments, but creating a project or deploying needs Edit)."))
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
	m.pickTarget, m.pickBack = t, m.screen
	m.pickItems, m.pickErr, m.pickCursor, m.pickLoading = nil, nil, 0, true
	m.screen = screenPicker
	return m, projectsCmd(t.Link.Platform)
}

// The picker's first row creates a new project; existing projects follow.
func (m Model) canCreate() bool { return deploy.CreatorFor(m.pickTarget.Link.Platform) != nil }

func (m Model) pickLines() []string {
	head := sTitle.Render("LINK PROJECT") + sDim.Render(fmt.Sprintf("  %s ← %s project", filepath.Base(m.pickTarget.Link.Project), m.pickTarget.Link.Platform))
	switch {
	case m.pickLoading:
		return []string{head, "", sDim.Render("  loading projects…")}
	case m.pickErr != nil:
		return []string{head, "", sBad.Render("  " + m.pickErr.Error())}
	}
	lines := []string{head, ""}
	row := func(i int, label string) string {
		if i == m.pickCursor {
			return sAccent.Render("▸ ") + label
		}
		return "  " + label
	}
	i := 0
	if m.canCreate() {
		lines = append(lines, row(i, sAccent.Render("+ Create a new project")+sDim.Render("  (first deploy)")))
		i++
	}
	if len(m.pickItems) == 0 {
		lines = append(lines, "", sDim.Render("  No existing projects on this "+m.pickTarget.Link.Platform+" account."))
	}
	for _, it := range m.pickItems {
		lines = append(lines, row(i, it.Name))
		i++
	}
	return lines
}

func (m Model) pickRows() int {
	n := len(m.pickItems)
	if m.canCreate() {
		n++
	}
	return n
}

// linkTo saves the binding, returns to where the picker was opened and refreshes.
func (m Model) linkTo(ref deploy.ProjectRef, verb string) (tea.Model, tea.Cmd) {
	t := m.pickTarget
	m.cfg.SetDeployLink(t.Link.Project, t.Link.Platform, ref.Ref, ref.Team)
	m.screen = m.pickBack
	m.creating, m.createConfirm = false, false
	if !m.cfg.Ephemeral {
		if err := m.cfg.Save(); err != nil {
			m.flash = verb + ", but couldn't save config: " + err.Error()
			m.keepFlash = m.flash
			return m, m.startRefresh()
		}
	}
	m.flash = verb + " " + ref.Name + " and linked " + filepath.Base(t.Link.Project)
	if verb == "created" {
		m.flash += " · press p to deploy"
	}
	m.keepFlash = m.flash
	return m, m.startRefresh()
}

func (m Model) handlePickerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.createConfirm {
		if k.String() == "y" {
			m.createConfirm = false
			m.flash = "creating " + m.createName + "…"
			return m, createCmd(m.pickTarget.Link.Platform, m.createName)
		}
		m.createConfirm = false
		m.flash = "cancelled, nothing created"
		return m, nil
	}
	if m.creating {
		switch k.String() {
		case "esc":
			m.creating = false
			m.createInput.Blur()
			return m, nil
		case "enter":
			name := strings.TrimSpace(m.createInput.Value())
			if !deploy.ValidName(name) {
				m.flash = "use lowercase letters, digits and hyphens (no leading or trailing hyphen)"
				return m, nil
			}
			m.creating, m.createName = false, name
			m.createInput.Blur()
			m.createConfirm = true
			return m, nil
		}
		var cmd tea.Cmd
		m.createInput, cmd = m.createInput.Update(k)
		return m, cmd
	}
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "h", "backspace", "left":
		m.screen = m.pickBack
	case "j", "down":
		m.pickCursor = min(m.pickCursor+1, max(0, m.pickRows()-1))
	case "k", "up":
		m.pickCursor = max(m.pickCursor-1, 0)
	case "n":
		if m.canCreate() && !m.pickLoading {
			return m.startCreate(), nil
		}
	case "enter":
		if m.pickLoading || m.pickErr != nil {
			break
		}
		i := m.pickCursor
		if m.canCreate() {
			if i == 0 {
				return m.startCreate(), nil
			}
			i--
		}
		if i >= 0 && i < len(m.pickItems) {
			return m.linkTo(m.pickItems[i], "linked")
		}
	}
	return m, nil
}

func (m Model) startCreate() Model {
	in := textinput.New()
	in.Prompt = sAccent.Render("new project name > ")
	in.CharLimit = 52
	in.SetValue(deploy.SuggestName(filepath.Base(m.pickTarget.Link.Project)))
	in.CursorEnd()
	in.Focus()
	m.createInput, m.creating = in, true
	return m
}

type createdMsg struct {
	platform string
	ref      deploy.ProjectRef
	err      error
}

func createCmd(platform, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ref, err := deploy.CreateProject(ctx, platform, name)
		return createdMsg{platform, ref, err}
	}
}

// --- deploy (publish) ---

// askDeploy shows what would run and waits for confirmation; nothing is
// executed until the user presses y.
func (m Model) askDeploy(t app.DeployTarget) (tea.Model, tea.Cmd) {
	cmd, err := deploy.DeployCommand(t.Link, t.Link.Project)
	if err != nil {
		m.flash = err.Error()
		return m, nil
	}
	if !cmd.Installed() {
		m.flash = cmd.Bin + " CLI not found: install it with `" + cmd.Install + "`, then press p again"
		return m, nil
	}
	m.deployAsk = &cmd
	return m, nil
}

func (m Model) handleDeployConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := m.deployAsk
	m.deployAsk = nil
	if k.String() != "y" {
		m.flash = "cancelled, nothing deployed"
		return m, nil
	}
	return m.beginDeploy(*cmd, m.deployPlatformFor(cmd))
}

// deployPlatformFor names the platform a CLI command belongs to.
func (m Model) deployPlatformFor(c *deploy.Command) string {
	switch c.Bin {
	case "wrangler":
		return "cloudflare"
	}
	return c.Bin
}
