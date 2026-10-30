package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Arnav717/oryn/internal/agents"
)

// reviewView shows what an agent changed.
type reviewView struct {
	run     agents.Run
	lines   []string
	summary string
	loading bool
	offset  int
}

type reviewMsg struct {
	slug    string
	changes agents.Changes
	err     error
}
type shippedMsg struct {
	url string
	err error
}
type discardedMsg struct {
	slug string
	err  error
}

type discardAsk struct {
	run   agents.Run
	input textinput.Model
}

// ownRun is the selected session's run. Sessions oryn didn't start have none.
func (m Model) ownRun() (agents.Run, bool) {
	s, ok := m.agSelected()
	if !ok || s.Run == nil {
		return agents.Run{}, false
	}
	return *s.Run, true
}

func (m Model) openReview() (tea.Model, tea.Cmd) {
	r, ok := m.ownRun()
	if !ok {
		m.flash = "oryn didn't start this one, so there's no worktree to review"
		return m, nil
	}
	m.review = &reviewView{run: r, loading: true}
	return m, func() tea.Msg {
		c, err := agents.Review(r)
		return reviewMsg{r.Slug, c, err}
	}
}

func (m Model) onReview(msg reviewMsg) (tea.Model, tea.Cmd) {
	if m.review == nil || m.review.run.Slug != msg.slug {
		return m, nil
	}
	v := *m.review
	v.loading = false
	if msg.err != nil {
		m.review, m.flash = nil, "couldn't read its changes: "+msg.err.Error()
		return m, nil
	}
	c := msg.changes
	parts := []string{fmt.Sprintf("%d commit(s)", c.Commits)}
	if c.Dirty > 0 {
		parts = append(parts, sWarn.Render(fmt.Sprintf("%d uncommitted", c.Dirty)))
	}
	v.summary = strings.Join(parts, sDim.Render(" · "))
	v.lines = diffLines(c)
	m.review = &v
	return m, nil
}

// diffLines colours a diff. It's text an agent wrote, so anything that could
// drive the terminal is stripped first.
func diffLines(c agents.Changes) []string {
	var out []string
	for _, f := range c.Untracked {
		out = append(out, sOK.Render("new file  "+clean(f)))
	}
	if len(c.Untracked) > 0 {
		out = append(out, "")
	}
	if c.Diff == "" {
		if len(out) == 0 {
			return []string{sDim.Render("No changes yet.")}
		}
		return out
	}
	for _, l := range strings.Split(c.Diff, "\n") {
		l = clean(l)
		switch {
		case strings.HasPrefix(l, "diff --git"):
			out = append(out, "", sBold.Render(strings.TrimPrefix(l, "diff --git ")))
		case strings.HasPrefix(l, "index "), strings.HasPrefix(l, "--- "), strings.HasPrefix(l, "+++ "):
		case strings.HasPrefix(l, "@@"):
			out = append(out, sAccent.Render(l))
		case strings.HasPrefix(l, "+"):
			out = append(out, sOK.Render(l))
		case strings.HasPrefix(l, "-"):
			out = append(out, sBad.Render(l))
		default:
			out = append(out, sDim.Render(l))
		}
	}
	return out
}

func clean(s string) string {
	s = strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func (m Model) reviewLines() []string {
	v := m.review
	head := sTitle.Render(v.run.Task) + "  " + sDim.Render(v.run.Branch)
	if v.summary != "" {
		head += "  " + v.summary
	}
	if v.loading {
		return []string{head, "", sDim.Render("reading changes…")}
	}
	lines := []string{head, ""}
	return append(lines, v.lines[min(v.offset, len(v.lines)):]...)
}

func (m Model) handleReviewKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := *m.review
	page := max(1, m.h-m.chrome()-4)
	last := max(0, len(v.lines)-page)
	switch k.String() {
	case "esc", "v":
		m.review = nil
		return m, nil
	case "q":
		return m, tea.Quit
	case "j", "down":
		v.offset++
	case "k", "up":
		v.offset--
	case "pgdown", "ctrl+d", " ":
		v.offset += page
	case "pgup", "ctrl+u":
		v.offset -= page
	case "g", "home":
		v.offset = 0
	case "G", "end":
		v.offset = last
	case "P":
		r := v.run
		m.shipAsk = &r
		return m, nil
	case "X":
		return m.startDiscard(v.run)
	case "a":
		r := v.run
		m.review = nil
		return m.attachRun(r)
	}
	v.offset = max(0, min(v.offset, last))
	m.review = &v
	return m, nil
}

func (m Model) askShip() (tea.Model, tea.Cmd) {
	r, ok := m.ownRun()
	if !ok {
		m.flash = "oryn didn't start this one, so it won't push it"
		return m, nil
	}
	m.shipAsk = &r
	return m, nil
}

func (m Model) handleShipConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := *m.shipAsk
	m.shipAsk = nil
	if k.String() != "y" {
		m.flash = "cancelled, nothing pushed"
		return m, nil
	}
	m.flash = "pushing " + r.Branch + "…"
	return m, func() tea.Msg {
		url, err := agents.Ship(r)
		return shippedMsg{url, err}
	}
}

func (m Model) onShipped(msg shippedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.flash = msg.err.Error()
		return m, nil
	}
	m.flash = "PR ready: " + msg.url
	return m, readAgents
}

func (m Model) askDiscard() (tea.Model, tea.Cmd) {
	r, ok := m.ownRun()
	if !ok {
		m.flash = "oryn didn't start this one, so it won't delete it"
		return m, nil
	}
	return m.startDiscard(r)
}

func (m Model) startDiscard(r agents.Run) (tea.Model, tea.Cmd) {
	in := textinput.New()
	in.Prompt = ""
	m.discard = &discardAsk{r, in}
	return m, m.discard.input.Focus()
}

func (m Model) handleDiscardKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := *m.discard
	switch k.Type {
	case tea.KeyEsc:
		m.discard = nil
		m.flash = "cancelled, nothing deleted"
		return m, nil
	case tea.KeyEnter:
		if d.input.Value() != d.run.Slug {
			m.discard = nil
			m.flash = "name didn't match, nothing deleted"
			return m, nil
		}
		m.discard, m.review, m.agOpen = nil, nil, ""
		r := d.run
		return m, func() tea.Msg { return discardedMsg{r.Slug, agents.Discard(r)} }
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(k)
	m.discard = &d
	return m, cmd
}

func (m Model) onDiscarded(msg discardedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.flash = "discard failed: " + msg.err.Error()
	} else {
		m.flash = "discarded " + msg.slug
	}
	return m, readAgents
}
