package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/notify"
	"github.com/Arnav717/oryn/internal/store"
)

// maxIndividual is how many alerts get their own notification; more than this
// collapse into one summary so adding a big project doesn't spam the desktop.
const maxIndividual = 3

// alertWorthy picks the events that deserve a desktop notification: high or
// critical vulnerabilities and any upstream status change. New releases are
// too frequent to interrupt anyone.
func alertWorthy(events []store.Event) []store.Event {
	var out []store.Event
	for _, e := range events {
		if (e.Kind == "vuln" && e.Severity >= 3) || e.Kind == "status" {
			out = append(out, e)
		}
	}
	return out
}

// notifications turns alerts into (title, body) pairs.
func notifications(alerts []store.Event) [][2]string {
	if len(alerts) == 0 {
		return nil
	}
	if len(alerts) <= maxIndividual {
		out := make([][2]string, len(alerts))
		for i, e := range alerts {
			out[i] = [2]string{"oryn: " + e.Kind, e.Title}
		}
		return out
	}
	var lines []string
	for _, e := range alerts[:maxIndividual] {
		lines = append(lines, e.Title)
	}
	lines = append(lines, fmt.Sprintf("…and %d more", len(alerts)-maxIndividual))
	return [][2]string{{fmt.Sprintf("oryn: %d new alerts", len(alerts)), strings.Join(lines, "\n")}}
}

// daemon refreshes in the background and sends desktop notifications for new
// alerts. Config is re-read every cycle, so `oryn add` takes effect without a
// restart.
func daemon(cfgPath, dbPath string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(dbPath)
	if err != nil {
		fatal(err)
	}
	defer st.Close()

	seen, err := st.HasEvents()
	if err != nil {
		fatal(err)
	}
	baseline := !seen // first ever run: record what exists without notifying
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf("oryn daemon %s started", version)

	for {
		wait := 10 * time.Minute
		cfg, err := config.Load(cfgPath)
		switch {
		case err != nil:
			log.Printf("config: %v", err)
		case len(cfg.Projects) == 0:
			log.Print("no watched projects; run `oryn add` in a project directory")
			wait = cfg.RefreshEvery
		default:
			wait = cfg.RefreshEvery
			snap := app.Refresh(ctx, cfg, st)
			for _, w := range snap.Warnings {
				log.Print("warning: ", w)
			}
			alerts := alertWorthy(snap.New)
			log.Printf("refreshed %d projects, %d deps: %d new events, %d alerts",
				len(cfg.Projects), len(snap.Deps), len(snap.New), len(alerts))
			if baseline {
				log.Print("first run: recorded a baseline, no notifications")
				baseline = false
			} else {
				for _, n := range notifications(alerts) {
					if err := notify.Send(n[0], n[1]); err != nil {
						log.Printf("notify: %v", err)
						break
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			log.Print("stopped")
			return
		case <-time.After(wait):
		}
	}
}
