package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/source"
)

// actionAsk is an outward action waiting for y/n; nothing runs until then.
type actionAsk struct {
	prompt string
	warn   []string
	done   string // flashed when it worked
	fail   string // flashed in front of the reason when it didn't
	run    func(context.Context) error
}

type actionDoneMsg struct {
	ask *actionAsk
	err error
}

func (m Model) ask(a actionAsk) (tea.Model, tea.Cmd) {
	if m.acting {
		m.flash = "still working on the last one"
		return m, nil
	}
	m.actAsk = &a
	return m, nil
}

func (m Model) handleActionConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	a := m.actAsk
	m.actAsk = nil
	if k.String() != "y" {
		m.flash = "cancelled, nothing changed"
		return m, nil
	}
	m.acting = true
	m.flash = "working…"
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return actionDoneMsg{a, a.run(ctx)}
	}
}

func (m Model) onActionDone(msg actionDoneMsg) (tea.Model, tea.Cmd) {
	m.acting = false
	m.flash = msg.ask.done
	if msg.err != nil {
		m.flash = msg.ask.fail + ": " + msg.err.Error()
	}
	m.keepFlash = m.flash
	return m, m.startRefresh() // so the list shows what just changed
}

func (m Model) actionPrompt() string {
	a := m.actAsk
	prompt := sWarn.Render(a.prompt + "  y to confirm, any other key cancels")
	if len(a.warn) > 0 {
		prompt = sBad.Render("WARNING: "+strings.Join(a.warn, "; ")+".  ") + prompt
	}
	return clip(prompt, m.w)
}

// selectedPR is the pull request under the cursor in the PRs section.
func (m Model) selectedPR(d *source.RepoData) (source.PR, bool) {
	c := m.rcur[svPRs]
	if d == nil || m.rsub != svPRs || c < 0 || c >= len(d.PRs) {
		return source.PR{}, false
	}
	return d.PRs[c], true
}

// selectedRun is the workflow run under the cursor in the Actions section.
func (m Model) selectedRun(d *source.RepoData) (source.Run, bool) {
	c := m.rcur[svActions]
	if d == nil || m.rsub != svActions || c < 0 || c >= len(d.Runs) {
		return source.Run{}, false
	}
	return d.Runs[c], true
}

func (m Model) askMerge(d *source.RepoData) (tea.Model, tea.Cmd) {
	p, ok := m.selectedPR(d)
	switch {
	case !ok:
		m.flash = "select a pull request in the PRs section"
		return m, nil
	case p.Draft:
		m.flash = "this PR is a draft, so it can't be merged yet"
		return m, nil
	case p.Mergeable == "CONFLICTING":
		m.flash = "this PR has conflicts to resolve first"
		return m, nil
	case p.CommitOID == "":
		m.flash = "no head commit known yet, press r and try again"
		return m, nil
	}
	var warn []string
	switch {
	case p.Failing():
		warn = append(warn, "CI is failing")
	case p.CI == "PENDING" || p.CI == "EXPECTED":
		warn = append(warn, "CI is still running")
	}
	switch p.Review {
	case "CHANGES_REQUESTED":
		warn = append(warn, "changes were requested")
	case "REVIEW_REQUIRED":
		warn = append(warn, "it isn't approved")
	}
	method := d.MergeMethod
	verb := map[string]string{"squash": "squash-merge", "rebase": "rebase-merge"}[method]
	if verb == "" {
		method, verb = "merge", "merge"
	}
	ref, n, sha := d.Ref, p.Number, p.CommitOID
	return m.ask(actionAsk{
		prompt: fmt.Sprintf("%s #%d by %s into %s?", verb, n, p.Author, orDash(p.Base)),
		warn:   warn,
		done:   fmt.Sprintf("merged #%d", n),
		fail:   fmt.Sprintf("couldn't merge #%d", n),
		run: func(ctx context.Context) error {
			return source.MergePR(ctx, ref, n, sha, method)
		},
	})
}

func (m Model) askApprove(d *source.RepoData) (tea.Model, tea.Cmd) {
	p, ok := m.selectedPR(d)
	switch {
	case !ok:
		m.flash = "select a pull request in the PRs section"
		return m, nil
	case p.Author == d.Viewer:
		m.flash = "GitHub doesn't let you approve your own PR"
		return m, nil
	case p.CommitOID == "":
		m.flash = "no head commit known yet, press r and try again"
		return m, nil
	}
	ref, n, sha := d.Ref, p.Number, p.CommitOID
	return m.ask(actionAsk{
		prompt: fmt.Sprintf("approve #%d by %s?", n, p.Author),
		done:   fmt.Sprintf("approved #%d", n),
		fail:   fmt.Sprintf("couldn't approve #%d", n),
		run: func(ctx context.Context) error {
			return source.ApprovePR(ctx, ref, n, sha)
		},
	})
}

func (m Model) askRerun(d *source.RepoData) (tea.Model, tea.Cmd) {
	r, ok := m.selectedRun(d)
	switch {
	case !ok:
		m.flash = "select a run in the Actions section"
		return m, nil
	case !r.Failing() && r.Conclusion != "cancelled":
		m.flash = "only failed or cancelled runs can be re-run"
		return m, nil
	}
	var warn []string
	if strings.HasPrefix(r.Event, "pull_request") && r.Actor != d.Viewer {
		warn = append(warn, "this runs code from a PR by "+r.Actor)
	}
	ref, id := d.Ref, r.ID
	return m.ask(actionAsk{
		prompt: fmt.Sprintf("re-run the failed jobs of run #%d?", r.Number),
		warn:   warn,
		done:   fmt.Sprintf("re-running run #%d", r.Number),
		fail:   fmt.Sprintf("couldn't re-run #%d", r.Number),
		run: func(ctx context.Context) error {
			return source.RerunFailed(ctx, ref, id)
		},
	})
}

func (m Model) openRunLog(d *source.RepoData) (tea.Model, tea.Cmd) {
	r, ok := m.selectedRun(d)
	if !ok {
		m.flash = "select a run in the Actions section"
		return m, nil
	}
	ref, id := d.Ref, r.ID
	return m.openLog(fmt.Sprintf("%s #%d", clean(r.Workflow), r.Number), func(ctx context.Context) logResult {
		lines, live, err := source.RunLog(ctx, ref, id)
		return logResult{lines, live, err}
	})
}
