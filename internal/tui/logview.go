package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const logPoll = 4 * time.Second

type logResult struct {
	lines []string
	live  bool // still being written, so ask again
	err   error
}

// logView is a CI or deploy log on its own screen. Logs are other people's
// text, so they stay here: they never reach oryn context, MCP or a Fix task.
type logView struct {
	title  string
	fetch  func(context.Context) logResult
	lines  []string
	off    int
	live   bool
	loaded bool
	err    error
}

type logMsg struct {
	view *logView
	res  logResult
}

type logTickMsg struct{ view *logView }

func fetchLog(v *logView) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return logMsg{v, v.fetch(ctx)}
	}
}

func (m Model) openLog(title string, fetch func(context.Context) logResult) (tea.Model, tea.Cmd) {
	m.logv = &logView{title: title, fetch: fetch}
	return m, fetchLog(m.logv)
}

func (m Model) logVisible() int { return max(1, m.h-m.chrome()-4) }

func (m Model) logEnd(v *logView) int { return max(0, len(v.lines)-m.logVisible()) }

// onLog keeps you at the end if that's where you were, so a live log follows
// and one you scrolled back through stays put.
func (m Model) onLog(msg logMsg) (tea.Model, tea.Cmd) {
	v := msg.view
	if v != m.logv {
		return m, nil
	}
	follow := !v.loaded || v.off >= m.logEnd(v)
	v.lines = v.lines[:0]
	for _, l := range msg.res.lines {
		v.lines = append(v.lines, clean(l))
	}
	v.live, v.err, v.loaded = msg.res.live, msg.res.err, true
	if follow {
		v.off = m.logEnd(v)
	}
	if !v.live {
		return m, nil
	}
	return m, tea.Tick(logPoll, func(time.Time) tea.Msg { return logTickMsg{v} })
}

func (m Model) logLines() []string {
	v := m.logv
	head := sTitle.Render(v.title)
	switch {
	case !v.loaded:
		head += sDim.Render("  loading…")
	case v.live:
		head += "  " + sAccent.Render("● live")
	}
	lines := []string{head, ""}
	if v.err != nil {
		lines = append(lines, sBad.Render(v.err.Error()))
	} else if v.loaded && len(v.lines) == 0 {
		lines = append(lines, sDim.Render("Nothing in the log yet."))
	}
	end := min(len(v.lines), v.off+m.logVisible())
	for _, l := range v.lines[min(v.off, end):end] {
		if rest, ok := strings.CutPrefix(l, "##[error]"); ok {
			l = sBad.Render(rest)
		}
		lines = append(lines, clip(l, m.w-4))
	}
	return lines
}

func (m Model) handleLogKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := m.logv
	switch k.String() {
	case "esc", "q", "backspace", "h", "left":
		m.logv = nil
		return m, nil
	case "j", "down":
		v.off++
	case "k", "up":
		v.off--
	case "pgdown", "ctrl+d":
		v.off += m.logVisible()
	case "pgup", "ctrl+u":
		v.off -= m.logVisible()
	case "g", "home":
		v.off = 0
	case "G", "end":
		v.off = m.logEnd(v)
	}
	v.off = max(0, min(v.off, m.logEnd(v)))
	return m, nil
}
