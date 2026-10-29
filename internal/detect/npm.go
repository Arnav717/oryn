package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type npmLockPkg struct {
	Version         string            `json:"version"`
	Dev             bool              `json:"dev"`
	Link            bool              `json:"link"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

type npmLock struct {
	LockfileVersion int                   `json:"lockfileVersion"`
	Packages        map[string]npmLockPkg `json:"packages"`
	// lockfileVersion 1 only
	Dependencies map[string]struct {
		Version string `json:"version"`
		Dev     bool   `json:"dev"`
	} `json:"dependencies"`
}

func parseNPMLock(path string) ([]Dep, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock npmLock
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	var deps []Dep
	seen := map[string]bool{}
	add := func(d Dep) {
		k := d.Name + "@" + d.Version
		if seen[k] {
			return
		}
		seen[k] = true
		deps = append(deps, d)
	}

	if len(lock.Packages) > 0 {
		root := lock.Packages[""]
		for key, p := range lock.Packages {
			i := strings.LastIndex(key, "node_modules/")
			if i < 0 || p.Link || p.Version == "" {
				continue // root or workspace entry
			}
			name := key[i+len("node_modules/"):]
			_, inDeps := root.Dependencies[name]
			_, inDev := root.DevDependencies[name]
			add(Dep{
				Ecosystem: NPM,
				Name:      name,
				Version:   p.Version,
				Direct:    key == "node_modules/"+name && (inDeps || inDev),
				Dev:       p.Dev,
			})
		}
		return deps, nil
	}

	for name, p := range lock.Dependencies {
		add(Dep{Ecosystem: NPM, Name: name, Version: p.Version, Dev: p.Dev})
	}
	return deps, nil
}
