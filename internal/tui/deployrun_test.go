package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/deploy"
)

func TestCleanLineAndURL(t *testing.T) {
	if got := cleanLine("\x1b[32mReady\x1b[0m  "); got != "Ready" {
		t.Errorf("cleanLine = %q", got)
	}
	lines := []string{
		"Inspect: https://vercel.com/team/proj/abc",
		`  "message": "Deployment x-abc.vercel.app ready. Production URL: https://portfolio-khaki-mu-11.vercel.app",`,
		"next: vercel curl https://portfolio-khaki-mu-11.vercel.app",
	}
	if got := findURL(lines); got != "https://portfolio-khaki-mu-11.vercel.app" {
		t.Errorf("findURL = %q", got)
	}
	if got := findURL([]string{"Deployed to https://abc123.portfolio.pages.dev."}); got != "https://abc123.portfolio.pages.dev" {
		t.Errorf("pages.dev url = %q", got)
	}
	if findURL([]string{"nothing here"}) != "" {
		t.Error("no URL expected")
	}
}

// drain runs a script through startRun and returns the lines and final event.
func drain(t *testing.T, script string) ([]string, runEvent) {
	t.Helper()
	ch := startRun(exec.CommandContext(context.Background(), "sh", "-c", script))
	var lines []string
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("channel closed without a done event")
			}
			if ev.done {
				return lines, ev
			}
			lines = append(lines, ev.line)
		case <-timeout:
			t.Fatal("timed out")
		}
	}
}

func TestStartRunStreamsOutput(t *testing.T) {
	lines, done := drain(t, `printf 'one\ntwo\r\033[32mthree\033[0m\n'; echo err >&2; exit 0`)
	if done.err != nil {
		t.Fatalf("err = %v", done.err)
	}
	if got := strings.Join(lines, "|"); got != "one|two|three|err" {
		t.Errorf("lines = %q", got)
	}
	_, done = drain(t, `echo boom >&2; exit 3`)
	if done.err == nil {
		t.Error("non-zero exit must be an error")
	}
	_, done = drain(t, `cat; echo after-stdin`) // stdin must not hang the process
	if done.err != nil {
		t.Errorf("closed stdin: %v", done.err)
	}
}

func TestRunFinishesWithResult(t *testing.T) {
	m := Model{cfg: nil, screen: screenDeployRun, now: time.Now()}
	m.run = &deployRun{started: time.Now().Add(-5 * time.Second), platform: "vercel", project: "/p/web",
		lines: []string{`"message": "ready. Production URL: https://web.vercel.app",`}}
	// startRefresh needs a config-free path: mark as already loading so it only queues.
	m.loading = true
	next, _ := m.onRunEvent(runEventMsg{m.run, runEvent{done: true}})
	got := next.(Model)
	if !got.run.done || got.run.err != nil || got.run.url != "https://web.vercel.app" || !strings.Contains(got.flash, "deployed vercel") {
		t.Fatalf("success: %+v flash=%q", got.run, got.flash)
	}
	if got.keepFlash == "" {
		t.Error("result must survive the refresh")
	}
	out := strings.Join(got.runLines(), "\n")
	if !strings.Contains(out, "deployed") || !strings.Contains(out, "LIVE AT") || !strings.Contains(out, "https://web.vercel.app") {
		t.Errorf("screen:\n%s", out)
	}

	// Exit 0 but the platform printed an error: still a failure.
	m.run = &deployRun{started: time.Now(), platform: "vercel", project: "/p/web", lines: []string{`  "status": "error",`}}
	next, _ = m.onRunEvent(runEventMsg{m.run, runEvent{done: true}})
	if got = next.(Model); got.run.err == nil || !strings.Contains(strings.Join(got.runLines(), "\n"), "failed") {
		t.Errorf("a printed error must count as failure: %+v", got.run)
	}

	// A stale run's events are ignored.
	old := &deployRun{started: time.Now()}
	if next, cmd := got.onRunEvent(runEventMsg{old, runEvent{line: "x"}}); cmd != nil || len(next.(Model).run.lines) != 1 {
		t.Error("stale run must be ignored")
	}
}

func TestRunKeys(t *testing.T) {
	cancelled := false
	m := Model{screen: screenDeployRun, runBack: screenDeploys}
	m.run = &deployRun{cancel: func() { cancelled = true }}
	next, _ := m.handleRunKey(keyMsg("esc"))
	if got := next.(Model); got.screen != screenDeployRun || !strings.Contains(got.flash, "still running") {
		t.Errorf("esc while running must not leave: %v %q", got.screen, got.flash)
	}
	next, _ = m.handleRunKey(keyMsg("x"))
	if !cancelled || !next.(Model).run.canceled {
		t.Error("x should cancel the process")
	}
	m.run.done = true
	next, _ = m.handleRunKey(keyMsg("enter"))
	if next.(Model).screen != screenDeploys {
		t.Error("enter after finishing returns to where the deploy started")
	}
}

var _ = deploy.Platforms
