package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/source"
)

// selectedRun is the workflow run under the cursor in the Actions section.
func (m Model) selectedRun(d *source.RepoData) (source.Run, bool) {
	c := m.rcur[svActions]
	if d == nil || m.rsub != svActions || c < 0 || c >= len(d.Runs) {
		return source.Run{}, false
	}
	return d.Runs[c], true
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
