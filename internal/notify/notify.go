// Package notify sends desktop notifications.
package notify

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

var ErrUnavailable = errors.New("no desktop notifier available (install libnotify's notify-send)")

// Send shows a desktop notification. It returns ErrUnavailable when the
// platform has no supported notifier.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "linux":
		if p, err := exec.LookPath("notify-send"); err == nil {
			return exec.Command(p, "--app-name=oryn", title, body).Run()
		}
	case "darwin":
		if p, err := exec.LookPath("osascript"); err == nil {
			script := `display notification "` + escape(body) + `" with title "` + escape(title) + `"`
			return exec.Command(p, "-e", script).Run()
		}
	}
	return ErrUnavailable
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
