// Package browser opens URLs in the user's default browser.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Open launches the default browser on an https URL. The URL is passed as an
// argument, never through a shell.
func Open(url string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing to open non-https URL %q", url)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("opening a browser is not supported on %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
