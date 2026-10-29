package source

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/mod/module"

	"github.com/vontrix/tape/internal/detect"
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
	out := map[string]string{}
	var mu sync.Mutex
	forEach(pkgs, 8, func(p pkg) {
		v, err := Latest(ctx, p.eco, p.name)
		if err != nil || v == "" {
			return
		}
		mu.Lock()
		out[PkgKey(p.eco, p.name)] = v
		mu.Unlock()
	})
	return out
}

// Outdated reports whether latest differs from the current version. Unpinned
// deps (no current version) are never outdated.
func Outdated(current, latest string) bool {
	return current != "" && latest != "" && strings.TrimPrefix(current, "v") != strings.TrimPrefix(latest, "v")
}
