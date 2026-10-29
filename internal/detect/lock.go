package detect

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	pypiSep = regexp.MustCompile(`[-_.]+`)
	reqName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)
)

// normPyPI applies PEP 503 name normalization.
func normPyPI(name string) string { return pypiSep.ReplaceAllString(strings.ToLower(name), "-") }

// parseCargoLock reads Cargo.lock. Packages without a source are workspace
// members; whatever they depend on is a direct dependency.
func parseCargoLock(path string) ([]Dep, error) {
	var lock struct {
		Package []struct {
			Name         string
			Version      string
			Source       string
			Dependencies []string
		}
	}
	if _, err := toml.DecodeFile(path, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	direct := map[string]bool{}
	for _, p := range lock.Package {
		if p.Source != "" {
			continue
		}
		for _, d := range p.Dependencies {
			if f := strings.Fields(d); len(f) > 0 {
				direct[f[0]] = true
			}
		}
	}
	var deps []Dep
	for _, p := range lock.Package {
		if !strings.HasPrefix(p.Source, "registry+") { // skip members, git and path deps
			continue
		}
		deps = append(deps, Dep{Ecosystem: Crates, Name: p.Name, Version: p.Version, Direct: direct[p.Name]})
	}
	return deps, nil
}

type uvDep struct{ Name string }

// parseUVLock reads uv.lock. Local packages (the project itself) define
// which registry packages are direct.
func parseUVLock(path string) ([]Dep, error) {
	var lock struct {
		Package []struct {
			Name                 string
			Version              string
			Source               map[string]any
			Dependencies         []uvDep
			DevDependencies      map[string][]uvDep `toml:"dev-dependencies"`
			OptionalDependencies map[string][]uvDep `toml:"optional-dependencies"`
		}
	}
	if _, err := toml.DecodeFile(path, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	prod, dev := map[string]bool{}, map[string]bool{}
	for _, p := range lock.Package {
		if _, registry := p.Source["registry"]; registry {
			continue
		}
		for _, d := range p.Dependencies {
			prod[normPyPI(d.Name)] = true
		}
		for _, group := range p.OptionalDependencies {
			for _, d := range group {
				prod[normPyPI(d.Name)] = true
			}
		}
		for _, group := range p.DevDependencies {
			for _, d := range group {
				dev[normPyPI(d.Name)] = true
			}
		}
	}
	var deps []Dep
	for _, p := range lock.Package {
		if _, registry := p.Source["registry"]; !registry {
			continue
		}
		n := normPyPI(p.Name)
		deps = append(deps, Dep{Ecosystem: PyPI, Name: n, Version: p.Version, Direct: prod[n] || dev[n], Dev: dev[n] && !prod[n]})
	}
	return deps, nil
}

// parsePoetryLock reads poetry.lock; direct deps come from the pyproject.toml beside it.
func parsePoetryLock(path string) ([]Dep, error) {
	var lock struct {
		Package []struct{ Name, Version string }
	}
	if _, err := toml.DecodeFile(path, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	prod, dev := readPyProject(filepath.Dir(path))
	deps := make([]Dep, 0, len(lock.Package))
	for _, p := range lock.Package {
		n := normPyPI(p.Name)
		deps = append(deps, Dep{Ecosystem: PyPI, Name: n, Version: p.Version, Direct: prod[n] || dev[n], Dev: dev[n] && !prod[n]})
	}
	return deps, nil
}

// readPyProject returns the normalized direct dependency names declared in
// pyproject.toml (PEP 621, PEP 735 groups and Poetry tables).
func readPyProject(dir string) (prod, dev map[string]bool) {
	prod, dev = map[string]bool{}, map[string]bool{}
	var py struct {
		Project struct {
			Dependencies         []string
			OptionalDependencies map[string][]string `toml:"optional-dependencies"`
		}
		DependencyGroups map[string][]any `toml:"dependency-groups"`
		Tool             struct {
			Poetry struct {
				Dependencies    map[string]any
				DevDependencies map[string]any `toml:"dev-dependencies"`
				Group           map[string]struct{ Dependencies map[string]any }
			}
		}
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "pyproject.toml"), &py); err != nil {
		return
	}
	req := func(set map[string]bool, s string) {
		if n := reqName.FindString(strings.TrimSpace(s)); n != "" {
			set[normPyPI(n)] = true
		}
	}
	for _, s := range py.Project.Dependencies {
		req(prod, s)
	}
	for _, group := range py.Project.OptionalDependencies {
		for _, s := range group {
			req(prod, s)
		}
	}
	for _, group := range py.DependencyGroups {
		for _, item := range group {
			if s, ok := item.(string); ok {
				req(dev, s)
			}
		}
	}
	for n := range py.Tool.Poetry.Dependencies {
		if n != "python" {
			prod[normPyPI(n)] = true
		}
	}
	for n := range py.Tool.Poetry.DevDependencies {
		dev[normPyPI(n)] = true
	}
	for _, g := range py.Tool.Poetry.Group {
		for n := range g.Dependencies {
			dev[normPyPI(n)] = true
		}
	}
	return
}
