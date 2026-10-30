package source

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/Arnav717/oryn/internal/detect"
)

// Latest returns the latest published version of a package.
func Latest(ctx context.Context, ecosystem, name string) (string, error) {
	switch ecosystem {
	case detect.NPM:
		var r struct {
			Version string `json:"version"`
		}
		err := getJSON(ctx, "https://registry.npmjs.org/"+name+"/latest", &r)
		return r.Version, err
	case detect.PyPI:
		var r struct {
			Info struct {
				Version string `json:"version"`
			} `json:"info"`
		}
		err := getJSON(ctx, "https://pypi.org/pypi/"+name+"/json", &r)
		return r.Info.Version, err
	case detect.Crates:
		return latestCrate(ctx, name)
	case detect.Go:
		esc, err := module.EscapePath(name)
		if err != nil {
			return "", err
		}
		var r struct {
			Version string `json:"Version"`
		}
		err = getJSON(ctx, "https://proxy.golang.org/"+esc+"/@latest", &r)
		return r.Version, err
	}
	return "", fmt.Errorf("unsupported ecosystem %q", ecosystem)
}

var (
	latestVersions = &diskCache[string]{name: "cache-latest.json", ttl: 6 * time.Hour}
	latestFn       = Latest
)

// LatestAll looks up latest versions for direct deps, keyed by PkgKey.
func LatestAll(ctx context.Context, deps []detect.Dep) map[string]string {
	type pkg struct{ eco, name string }
	var pkgs []pkg
	seen := map[string]bool{}
	for _, d := range deps {
		k := PkgKey(d.Ecosystem, d.Name)
		if !d.Direct || seen[k] {
			continue
		}
		seen[k] = true
		pkgs = append(pkgs, pkg{d.Ecosystem, d.Name})
	}
	now := time.Now()
	out := map[string]string{}
	var ask []pkg
	for _, p := range pkgs {
		v, found, fresh := latestVersions.get(PkgKey(p.eco, p.name), now)
		if found {
			out[PkgKey(p.eco, p.name)] = v // kept if the fresh lookup fails
		}
		if !fresh {
			ask = append(ask, p)
		}
	}
	var mu sync.Mutex
	forEach(ask, 16, func(p pkg) {
		v, err := latestFn(ctx, p.eco, p.name)
		if err != nil || v == "" {
			return
		}
		mu.Lock()
		out[PkgKey(p.eco, p.name)] = v
		mu.Unlock()
		latestVersions.put(PkgKey(p.eco, p.name), v, now)
	})
	if len(ask) > 0 {
		latestVersions.save()
	}
	return out
}

// Outdated reports whether latest differs from the current version. Unpinned
// deps (no current version) are never outdated.
func Outdated(current, latest string) bool {
	return current != "" && latest != "" && strings.TrimPrefix(current, "v") != strings.TrimPrefix(latest, "v")
}

// crateIndexPath is the sparse-index path for a crate name.
func crateIndexPath(name string) string {
	n := strings.ToLower(name)
	switch len(n) {
	case 0:
		return ""
	case 1, 2:
		return fmt.Sprintf("%d/%s", len(n), n)
	case 3:
		return "3/" + n[:1] + "/" + n
	}
	return n[:2] + "/" + n[2:4] + "/" + n
}

// latestCrate returns the highest non-yanked, non-prerelease version from the
// crates.io sparse index (one JSON object per line, one per version).
func latestCrate(ctx context.Context, name string) (string, error) {
	path := crateIndexPath(name)
	if path == "" {
		return "", fmt.Errorf("empty crate name")
	}
	body, err := getText(ctx, "https://index.crates.io/"+path)
	if err != nil {
		return "", err
	}
	best := ""
	for _, line := range strings.Split(body, "\n") {
		var v struct {
			Vers   string `json:"vers"`
			Yanked bool   `json:"yanked"`
		}
		if line == "" || json.Unmarshal([]byte(line), &v) != nil || v.Yanked {
			continue
		}
		sv := "v" + v.Vers
		if !semver.IsValid(sv) || semver.Prerelease(sv) != "" {
			continue
		}
		if best == "" || semver.Compare(sv, "v"+best) > 0 {
			best = v.Vers
		}
	}
	if best == "" {
		return "", fmt.Errorf("no stable release of %s", name)
	}
	return best, nil
}
