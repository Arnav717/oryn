package tui

import (
	"errors"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/browser"
	"github.com/Arnav717/oryn/internal/source"
)

type ghSection struct {
	title string
	prs   []source.PR
}

// attention ranks your own PRs by how much they need you: red CI first, then
// requested changes, then everything else.
func attention(p source.PR) int {
	switch {
	case p.Failing():
		return 0
	case p.Review == "CHANGES_REQUESTED":
		return 1
	}
	return 2
}

func (m *Model) ghSections() []ghSection {
	if m.snap == nil || m.snap.GitHub == nil {
		return nil
	}
	gh := m.snap.GitHub
	mine := append([]source.PR(nil), gh.Mine...)
	sort.SliceStable(mine, func(i, j int) bool { return attention(mine[i]) < attention(mine[j]) })
	return []ghSection{
		{"REVIEW REQUESTED", gh.Review},
		{"MY PULL REQUESTS", mine},
	}
}

// ghPRs is the flat, cursor-addressable list in display order.
func (m *Model) ghPRs() []source.PR {
	var out []source.PR
	for _, s := range m.ghSections() {
		out = append(out, s.prs...)
	}
	return out
}

// ghSummary counts for the home row and header.
func (m *Model) ghSummary() (review, mine, failing int) {
	if m.snap == nil || m.snap.GitHub == nil {
		return
	}
	for _, p := range m.snap.GitHub.Mine {
		if p.Failing() {
			failing++
		}
	}
	return len(m.snap.GitHub.Review), len(m.snap.GitHub.Mine), failing
}

func (m *Model) ghClamp() {
	n := len(m.ghPRs())
	m.ghCursor = max(0, min(m.ghCursor, n-1))
	if m.h == 0 {
		return
	}
	_, line, top := m.ghLines()
	vis := max(1, m.h-m.chrome()-2)
	if top < m.ghOffset {
		m.ghOffset = top
	}
	if line >= m.ghOffset+vis {
		m.ghOffset = line - vis + 1
	}
	m.ghOffset = max(0, m.ghOffset)
}

func (m Model) handleGHKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "h", "backspace", "left":
		m.screen = screenHome
	case "?":
		m.showHelp = true
	case "r":
		return m, m.startRefresh()
	case "j", "down":
		m.ghCursor++
	case "k", "up":
		m.ghCursor--
	case "pgdown", "ctrl+d":
		m.ghCursor += max(1, m.h-m.chrome()-2)
	case "pgup", "ctrl+u":
		m.ghCursor -= max(1, m.h-m.chrome()-2)
	case "g", "home":
		m.ghCursor = 0
	case "G", "end":
		m.ghCursor = 1 << 30
	case "enter", "o", "l", "right":
		if prs := m.ghPRs(); m.ghCursor >= 0 && m.ghCursor < len(prs) {
			if err := browser.Open(prs[m.ghCursor].URL); err != nil {
				m.flash = err.Error()
			} else {
				m.flash = "opened " + prs[m.ghCursor].Ref()
			}
		}
	}
	m.ghClamp()
	return m, nil
}

// ghLines renders the screen body. It also reports the line holding the
// cursor and the first line of that row's block (its section header when the
// row leads a section), so scrolling can keep both in view.
func (m *Model) ghLines() (lines []string, cursorLine, cursorTop int) {
	switch {
	case m.snap == nil:
		return []string{sDim.Render("loading…")}, 0, 0
	case errors.Is(m.snap.GitHubErr, source.ErrNoToken):
		return []string{
			sTitle.Render("GITHUB") + sDim.Render("  not connected"),
			"",
			"  oryn reads your PRs through the GitHub CLI login or a token in the environment.",
			"  Run " + sAccent.Render("gh auth login") + " (or set GH_TOKEN), then press " + sAccent.Render("r") + ".",
			"  Nothing is stored by oryn; the token only goes to api.github.com.",
		}, 0, 0
	case m.snap.GitHubErr != nil:
		return []string{sTitle.Render("GITHUB"), "", "  " + sBad.Render(m.snap.GitHubErr.Error())}, 0, 0
	case m.snap.GitHub == nil:
		return []string{sTitle.Render("GITHUB"), "", sDim.Render("  disabled in config (github = false)")}, 0, 0
	}

	cw := m.w - 2
	const refW, authorW, ciW, reviewW, ageW = 34, 16, 4, 14, 5
	titleW := max(10, cw-2-refW-authorW-ciW-reviewW-ageW)

	review, mine, failing := m.ghSummary()
	head := sTitle.Render("GITHUB") + sDim.Render("  @"+m.snap.GitHub.Login+fmt.Sprintf("  %d to review · %d open", review, mine))
	if failing > 0 {
		head += sBad.Render(fmt.Sprintf(" · %d failing CI", failing))
	}
	lines = []string{head}

	idx := 0
	for _, sec := range m.ghSections() {
		lines = append(lines, "", sTitle.Render(sec.title)+sDim.Render(fmt.Sprintf("  %d", len(sec.prs))))
		if len(sec.prs) == 0 {
			lines = append(lines, "  "+sDim.Render("nothing here"))
			continue
		}
		lines = append(lines, sDim.Render("  "+pad("PR", refW)+pad("TITLE", titleW)+pad("AUTHOR", authorW)+pad("CI", ciW)+pad("REVIEW", reviewW)+"AGE"))
		for i, p := range sec.prs {
			marker := "  "
			if idx == m.ghCursor {
				marker = sAccent.Render("▸ ")
				cursorLine = len(lines)
				cursorTop = cursorLine
				switch {
				case idx == 0:
					cursorTop = 0 // first row: show the summary line too
				case i == 0:
					cursorTop -= 3 // blank line, section title, column header
				}
			}
			ref := p.Ref()
			if idx == m.ghCursor {
				ref = sBold.Render(ref)
			}
			lines = append(lines, marker+pad(ref, refW)+pad(p.Title, titleW)+pad(sDim.Render(p.Author), authorW)+
				pad(ciCell(p), ciW)+pad(reviewCell(p), reviewW)+sDim.Render(humanAge(p.Updated)))
			idx++
		}
	}
	return lines, cursorLine, max(0, cursorTop)
}

func ciCell(p source.PR) string {
	switch p.CI {
	case "SUCCESS":
		return sOK.Render("✓")
	case "FAILURE", "ERROR":
		return sBad.Render("✗")
	case "PENDING", "EXPECTED":
		return sWarn.Render("●")
	}
	return sDim.Render("-")
}

func reviewCell(p source.PR) string {
	switch {
	case p.Draft:
		return sDim.Render("draft")
	case p.Review == "APPROVED":
		return sOK.Render("approved")
	case p.Review == "CHANGES_REQUESTED":
		return sBad.Render("changes")
	case p.Review == "REVIEW_REQUIRED":
		return sDim.Render("needs review")
	}
	return sDim.Render("-")
}

func humanAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
}

// ghHomeSummary is the one-line GitHub status on the home tab.
func (m *Model) ghHomeSummary() string {
	switch {
	case m.snap == nil:
		return sDim.Render("…")
	case errors.Is(m.snap.GitHubErr, source.ErrNoToken):
		return sDim.Render("not connected · run `gh auth login`")
	case m.snap.GitHubErr != nil:
		return sBad.Render("error · open for details")
	case m.snap.GitHub == nil:
		return sDim.Render("disabled")
	}
	review, mine, failing := m.ghSummary()
	rv := sDim.Render(fmt.Sprintf("%d to review", review))
	if review > 0 {
		rv = sAccent.Render(fmt.Sprintf("%d to review", review))
	}
	out := rv + sDim.Render(fmt.Sprintf(" · %d open PRs", mine))
	if failing > 0 {
		out += sDim.Render(" · ") + sBad.Render(fmt.Sprintf("%d failing CI", failing))
	}
	return out
}
