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
	"github.com/Arnav717/oryn/internal/deploy"
)

// deleteAsk is a destructive action waiting for confirmation. Nothing is sent
// to the platform until the user confirms.
type deleteAsk struct {
	project bool // delete the whole project, else a single deployment
	target  app.DeployTarget
	dep     deploy.Deployment
	word    string // what must be typed to delete a project
	input   textinput.Model
}

type deletedMsg struct {
	ask deleteAsk
	err error
}

// projectWord is what the user types to confirm deleting a project: its name
// on the platform when known.
func projectWord(l deploy.Link) string {
	if l.Name != "" {
		return l.Name
	}
	return "delete"
}

func (m Model) askDeleteDeployment() (tea.Model, tea.Cmd) {
	_, rows, _ := m.deployRows()
	if m.dpCursor < 0 || m.dpCursor >= len(rows) || rows[m.dpCursor].dep == nil {
		m.flash = "select a deployment to delete"
		return m, nil
	}
	r := rows[m.dpCursor]
	t := m.snap.Deploys[r.target]
	if deploy.RemoverFor(t.Link.Platform) == nil {
		m.flash = "deleting is not supported on " + t.Link.Platform
		return m, nil
	}
	// Cloudflare always refuses this one, so don't ask only to fail.
	if t.Link.Platform == "cloudflare" && t.IsLive(*r.dep) {
		m.flash = "Cloudflare won't delete the live production deployment. Deploy a newer one first, or press X to delete the whole project"
		return m, nil
	}
	m.delAsk = &deleteAsk{target: t, dep: *r.dep}
	return m, nil
}

func (m Model) askDeleteProject() (tea.Model, tea.Cmd) {
	t, ok := m.deployTargetAtCursor()
	switch {
	case !ok:
		m.flash = "select a project to delete"
		return m, nil
	case !t.Link.Linked():
		m.flash = "this folder isn't linked to a " + t.Link.Platform + " project"
		return m, nil
	case deploy.RemoverFor(t.Link.Platform) == nil:
		m.flash = "deleting is not supported on " + t.Link.Platform
		return m, nil
	}
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 64
	in.Focus()
	m.delAsk = &deleteAsk{project: true, target: t, word: projectWord(t.Link), input: in}
	return m, nil
}

// unlink forgets the folder's link in oryn only; nothing is deleted remotely.
func (m Model) unlink() (tea.Model, tea.Cmd) {
	t, ok := m.deployTargetAtCursor()
	if !ok || !m.cfg.RemoveDeployLink(t.Link.Project, t.Link.Platform) {
		m.flash = "no oryn link to remove here (detected from a config file)"
		return m, nil
	}
	if !m.cfg.Ephemeral {
		if err := m.cfg.Save(); err != nil {
			m.flash = "unlinked, but couldn't save config: " + err.Error()
			return m, m.startRefresh()
		}
	}
	m.flash = "unlinked " + filepath.Base(t.Link.Project) + " from " + t.Link.Platform + " (the project still exists there)"
	m.keepFlash = m.flash
	return m, m.startRefresh()
}

func (m Model) handleDeleteKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	ask := m.delAsk
	if !ask.project {
		m.delAsk = nil
		if k.String() != "y" {
			m.flash = "cancelled, nothing deleted"
			return m, nil
		}
		m.flash = "deleting deployment…"
		return m, deleteCmd(*ask)
	}
	switch k.String() {
	case "esc":
		m.delAsk = nil
		m.flash = "cancelled, nothing deleted"
		return m, nil
	case "enter":
		if strings.TrimSpace(ask.input.Value()) != ask.word {
			m.flash = "that doesn't match; type '" + ask.word + "' exactly, or esc to cancel"
			return m, nil
		}
		m.delAsk = nil
		m.flash = "deleting project…"
		return m, deleteCmd(*ask)
	}
	var cmd tea.Cmd
	ask.input, cmd = ask.input.Update(k)
	return m, cmd
}

func deleteCmd(ask deleteAsk) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rm := deploy.RemoverFor(ask.target.Link.Platform)
		if rm == nil {
			return deletedMsg{ask, fmt.Errorf("deleting is not supported on %s", ask.target.Link.Platform)}
		}
		if ask.project {
			return deletedMsg{ask, rm.DeleteProject(ctx, ask.target.Link)}
		}
		return deletedMsg{ask, rm.DeleteDeployment(ctx, ask.target.Link, ask.dep.ID)}
	}
}

func (m Model) onDeleted(msg deletedMsg) (tea.Model, tea.Cmd) {
	a := msg.ask
	if msg.err != nil {
		m.flash = "delete failed: " + msg.err.Error()
		return m, nil
	}
	if a.project {
		m.cfg.RemoveDeployLink(a.target.Link.Project, a.target.Link.Platform)
		if !m.cfg.Ephemeral {
			m.cfg.Save()
		}
		m.flash = "deleted " + a.target.Link.Platform + " project " + projectWord(a.target.Link) + " and unlinked " + filepath.Base(a.target.Link.Project)
	} else {
		m.flash = "deleted " + a.target.Link.Platform + " deployment " + a.dep.ID
	}
	m.dpCursor = 0
	m.keepFlash = m.flash
	return m, m.startRefresh()
}

// deletePrompt is the footer text while a delete waits for confirmation.
func (m Model) deletePrompt() string {
	a := m.delAsk
	if a.project {
		return sBad.Render("DELETE "+a.target.Link.Platform+" project '"+a.word+"' and ALL its deployments, permanently. Type '"+a.word+"' + enter (esc cancels) > ") + a.input.View()
	}
	live := ""
	if a.dep.Target == "production" {
		live = " (production)"
	}
	return sBad.Render(fmt.Sprintf("delete %s deployment %s%s on %s? ", a.target.Link.Platform, shortID(a.dep.ID), live, a.dep.Branch)) +
		sWarn.Render("y to confirm, any other key cancels")
}

func shortID(id string) string {
	if len(id) > 14 {
		return id[:14] + "…"
	}
	return id
}
