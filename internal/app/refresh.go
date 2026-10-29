// Package app ties detectors, sources and the event store into one snapshot.
package app

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/vontrix/tape/internal/config"
	"github.com/vontrix/tape/internal/detect"
	"github.com/vontrix/tape/internal/source"
	"github.com/vontrix/tape/internal/store"
)

type Snapshot struct {
	Deps     []detect.Dep
	Vulns    map[string][]source.Vuln // by source.DepKey
	Latest   map[string]string        // by source.PkgKey
	Status   []source.Status
	Events   []store.Event
	Warnings []string
	At       time.Time
}

// VulnList flattens vulns, most severe first.
func (s *Snapshot) VulnList() []source.Vuln {
	var out []source.Vuln
	for _, vs := range s.Vulns {
		out = append(out, vs...)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := source.SeverityRank(out[i].Severity), source.SeverityRank(out[j].Severity)
		if ri != rj {
			return ri > rj
		}
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func Refresh(ctx context.Context, cfg *config.Config, st *store.Store) *Snapshot {
	snap := &Snapshot{At: time.Now()}
	for _, p := range cfg.Projects {
		deps, errs := detect.Scan(p)
		snap.Deps = append(snap.Deps, deps...)
		for _, err := range errs {
			snap.Warnings = append(snap.Warnings, "scan: "+err.Error())
		}
	}

	var wg sync.WaitGroup
	var vulnErr error
	wg.Add(3)
	go func() { defer wg.Done(); snap.Vulns, vulnErr = source.QueryOSV(ctx, snap.Deps) }()
	go func() { defer wg.Done(); snap.Latest = source.LatestAll(ctx, snap.Deps) }()
	go func() { defer wg.Done(); snap.Status = source.CheckStatus(ctx, cfg.Services) }()
	wg.Wait()
	if vulnErr != nil {
		snap.Warnings = append(snap.Warnings, "osv: "+vulnErr.Error())
	}

	if st != nil {
		if err := recordEvents(st, snap); err != nil {
			snap.Warnings = append(snap.Warnings, "store: "+err.Error())
		}
		evs, err := st.Recent(200)
		if err != nil {
			snap.Warnings = append(snap.Warnings, "store: "+err.Error())
		}
		snap.Events = evs
	}
	return snap
}

// recordEvents turns snapshot facts into tape events. The store dedupes, so
// only things not seen before show up as new.
func recordEvents(st *store.Store, snap *Snapshot) error {
	for _, v := range snap.VulnList() {
		_, err := st.Record(store.Event{
			Key:      fmt.Sprintf("vuln:%s:%s:%s@%s", v.ID, v.Ecosystem, v.Package, v.Version),
			Kind:     "vuln",
			Severity: source.SeverityRank(v.Severity),
			Entity:   source.PkgKey(v.Ecosystem, v.Package),
			Title:    fmt.Sprintf("%s %s in %s@%s: %s", v.Severity, v.CVE(), v.Package, v.Version, v.Summary),
			URL:      v.URL,
		})
		if err != nil {
			return err
		}
	}

	seen := map[string]bool{}
	for _, d := range snap.Deps {
		k := source.PkgKey(d.Ecosystem, d.Name)
		latest := snap.Latest[k]
		if !d.Direct || seen[k] || !source.Outdated(d.Version, latest) {
			continue
		}
		seen[k] = true
		_, err := st.Record(store.Event{
			Key:    "release:" + k + "@" + latest,
			Kind:   "release",
			Entity: k,
			Title:  fmt.Sprintf("%s %s available (you have %s)", d.Name, latest, d.Version),
		})
		if err != nil {
			return err
		}
	}

	for _, s := range snap.Status {
		if s.Err != nil {
			continue
		}
		title := fmt.Sprintf("%s: %s", s.Service, s.Description)
		last, err := st.LastTitle("status", s.Service)
		if err != nil {
			return err
		}
		// Record changes, but don't flood the tape with "operational" on first run.
		if title == last || (last == "" && s.OK()) {
			continue
		}
		sev := 0
		switch s.Indicator {
		case "minor", "maintenance":
			sev = 2
		case "major":
			sev = 3
		case "critical":
			sev = 4
		}
		_, err = st.Record(store.Event{
			Key:      fmt.Sprintf("status:%s:%d", s.Service, time.Now().UnixNano()),
			Kind:     "status",
			Severity: sev,
			Entity:   s.Service,
			Title:    title,
			URL:      s.URL,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
