package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Arnav717/oryn/internal/browser"
	"github.com/Arnav717/oryn/internal/deploy"
)

// runEvent is one thing a running deploy reports: a line of output, or the end.
type runEvent struct {
	line string
	done bool
	err  error
}

type runEventMsg struct {
	run *deployRun
	ev  runEvent
}

// deployRun is a deploy in progress or just finished, shown on its own screen.
type deployRun struct {
	platform, project, display string
	started, ended             time.Time
	lines                      []string
	done                       bool
	err                        error
	url                        string
	cancel                     context.CancelFunc
	canceled                   bool
	events                     chan runEvent
}

func (r *deployRun) running() bool { return !r.done }

func (r *deployRun) elapsed(now time.Time) time.Duration {
	if r.done {
		return r.ended.Sub(r.started)
	}
	return now.Sub(r.started)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)

func cleanLine(s string) string { return strings.TrimRight(ansiRE.ReplaceAllString(s, ""), " \t") }

// splitLines splits on \n and \r so progress output that rewrites a line
// shows up as separate updates instead of one huge line.
func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

var (
	prodURLRE = regexp.MustCompile(`Production(?: URL)?:\s*(https://[^\s"',)]+)`)
	anyURLRE  = regexp.MustCompile(`https://[A-Za-z0-9._-]+\.(?:vercel\.app|pages\.dev|netlify\.app|netlify\.live)[^\s"',)]*`)
)

// findURL picks the site URL out of the CLI's output: an explicit production
// URL if one is printed, otherwise the last platform URL seen.
func findURL(lines []string) string {
	text := strings.Join(lines, "\n")
	if m := prodURLRE.FindAllStringSubmatch(text, -1); len(m) > 0 {
		return strings.TrimRight(m[len(m)-1][1], ".")
	}
	if m := anyURLRE.FindAllString(text, -1); len(m) > 0 {
		return strings.TrimRight(m[len(m)-1], ".")
	}
	return ""
}

// outputFailed spots CLIs that print a failure but still exit 0.
func outputFailed(lines []string) string {
	for _, l := range lines {
		if strings.Contains(l, `"status": "error"`) {
			return "the platform reported an error"
		}
	}
	return ""
}

// startRun launches the command and streams its output on the returned
// channel, which is closed after the final done event.
func startRun(cmd *exec.Cmd) chan runEvent {
	ch := make(chan runEvent, 64)
	go func() {
		defer close(ch)
		pr, pw := io.Pipe()
		cmd.Stdout, cmd.Stderr = pw, pw
		if err := cmd.Start(); err != nil {
			ch <- runEvent{done: true, err: err}
			return
		}
		waitErr := make(chan error, 1)
		go func() { err := cmd.Wait(); pw.Close(); waitErr <- err }()
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		sc.Split(splitLines)
		for sc.Scan() {
			if l := cleanLine(sc.Text()); l != "" {
				ch <- runEvent{line: l}
			}
		}
		ch <- runEvent{done: true, err: <-waitErr}
	}()
	return ch
}

// waitRun blocks for the next event of a run and delivers it to Update.
func waitRun(r *deployRun) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-r.events
		if !ok {
			ev = runEvent{done: true}
		}
		return runEventMsg{r, ev}
	}
}

// beginDeploy starts the confirmed command and switches to the progress screen.
func (m Model) beginDeploy(c deploy.Command, platform string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &deployRun{platform: platform, project: c.Dir, display: c.Display(), started: time.Now(), cancel: cancel}
	r.events = startRun(c.Cmd(ctx))
	m.run, m.runBack = r, m.screen
	m.screen = screenDeployRun
	return m, waitRun(r)
}

func (m Model) onRunEvent(msg runEventMsg) (tea.Model, tea.Cmd) {
	r := msg.run
	if r != m.run { // an old run; ignore
		return m, nil
	}
	if !msg.ev.done {
		r.lines = append(r.lines, msg.ev.line)
		if len(r.lines) > 500 {
			r.lines = r.lines[len(r.lines)-500:]
		}
		return m, waitRun(r)
	}
	r.done, r.ended, r.err = true, time.Now(), msg.ev.err
	if r.err == nil {
		if why := outputFailed(r.lines); why != "" {
			r.err = fmt.Errorf("%s", why)
		}
	}
	r.url = findURL(r.lines)
	switch {
	case r.canceled:
		m.flash = "deploy cancelled"
	case r.err != nil:
		m.flash = r.platform + " deploy failed"
	default:
		m.flash = "deployed " + r.platform
		if r.url != "" {
			m.flash += " · " + r.url
		}
	}
	m.keepFlash = m.flash
	return m, m.startRefresh() // so the new deployment shows up in the list
}

var runFrames = []string{"◐", "◓", "◑", "◒"}

func mmss(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func (m Model) runLines() []string {
	r := m.run
	if r == nil {
		return nil
	}
	var status string
	switch {
	case r.running():
		status = sAccent.Render(runFrames[m.now.Second()%len(runFrames)]) + " " + sBold.Render("deploying") + sDim.Render("  "+mmss(r.elapsed(m.now)))
	case r.canceled:
		status = sWarn.Render("■ cancelled") + sDim.Render("  after "+mmss(r.elapsed(m.now)))
	case r.err != nil:
		status = sBad.Render("✗ failed") + sDim.Render("  after "+mmss(r.elapsed(m.now))+" · "+r.err.Error())
	default:
		status = sOK.Render("✓ deployed") + sDim.Render("  in "+mmss(r.elapsed(m.now)))
	}
	lines := []string{status, sDim.Render(r.display)}
	if r.done && r.err == nil && r.url != "" {
		lines = append(lines, "", sTitle.Render("Live at ")+r.url+sDim.Render("   (o to open)"))
	}
	lines = append(lines, "", sDim.Render("output"))
	room := max(3, m.h-m.chrome()-2-len(lines)-1)
	tail := r.lines[max(0, len(r.lines)-room):]
	if len(r.lines) == 0 {
		tail = []string{sDim.Render("  waiting for output…")}
	}
	for _, l := range tail {
		lines = append(lines, "  "+l)
	}
	return lines
}

func (m Model) handleRunKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := m.run
	if r == nil {
		m.screen = m.runBack
		return m, nil
	}
	if r.running() {
		switch k.String() {
		case "x":
			r.canceled = true
			r.cancel()
			m.flash = "cancelling…"
		case "esc", "q", "enter", "h":
			m.flash = "deploy still running: x cancels it, ctrl+c quits"
		}
		return m, nil
	}
	switch k.String() {
	case "o":
		if r.url == "" {
			m.flash = "no URL found in the output"
		} else if err := browser.Open(r.url); err != nil {
			m.flash = err.Error()
		} else {
			m.flash = "opened " + r.url
		}
	case "q":
		return m, tea.Quit
	case "esc", "enter", "h", "backspace", "left":
		m.screen = m.runBack
	}
	return m, nil
}
