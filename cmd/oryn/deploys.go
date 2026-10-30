package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/deploy"
)

func deploys(cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	found := false
	for _, p := range cfg.Projects {
		links := app.LinksFor(cfg, p)
		if len(links) == 0 {
			continue
		}
		found = true
		fmt.Println(config.ShortHome(p))
		for _, l := range links {
			prov := deploy.ProviderFor(l.Platform)
			switch {
			case prov == nil:
				fmt.Printf("  %-10s detected via %s (not queryable yet)\n", l.Platform, l.Marker)
				continue
			case !l.Linked():
				fmt.Printf("  %-10s detected via %s, not linked (run the platform CLI's link command)\n", l.Platform, l.Marker)
				continue
			}
			ds, err := prov.List(ctx, l, 5)
			switch {
			case errors.Is(err, deploy.ErrNoCredentials):
				fmt.Printf("  %-10s linked, no credentials (set the platform token or log in with its CLI)\n", l.Platform)
			case err != nil:
				fmt.Printf("  %-10s error: %v\n", l.Platform, err)
			default:
				fmt.Printf("  %-10s %s\n", l.Platform, l.Ref)
				for _, d := range ds {
					fmt.Printf("    %-8s %-10s %-14s %-8s %s  %s\n", d.State, d.Target, d.Branch, d.Commit, d.Created.Format("Jan02 15:04"), d.Message)
				}
			}
		}
	}
	if !found {
		fmt.Println("no deployment config found in watched projects")
	}
}
