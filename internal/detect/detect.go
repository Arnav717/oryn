// Package detect finds dependencies by parsing lockfiles and manifests.
package detect

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// Ecosystem names match OSV's ecosystem identifiers.
const (
	NPM    = "npm"
	Go     = "Go"
	PyPI   = "PyPI"
	Crates = "crates.io"
)

type Dep struct {
	Ecosystem string
	Name      string
	Version   string
	Direct    bool
	Dev       bool
	Project   string // project root that was scanned
	Manifest  string // file the dep was found in
}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true, ".venv": true, "venv": true,
	"target": true, "dist": true, "build": true, ".next": true, "__pycache__": true,
}

const maxDepth = 4

type parser func(path string) ([]Dep, error)

func parserFor(name string) parser {
	switch {
	case name == "package-lock.json":
		return parseNPMLock
	case name == "pnpm-lock.yaml":
		return parsePNPMLock
	case name == "yarn.lock":
		return parseYarnLock
	case name == "go.mod":
		return parseGoMod
	case name == "Cargo.lock":
		return parseCargoLock
	case name == "uv.lock":
		return parseUVLock
	case name == "poetry.lock":
		return parsePoetryLock
	case strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt"):
		return parseRequirements
	}
	return nil
}

// Scan walks root (bounded depth) and returns every dependency found.
// Per-file parse errors are returned alongside the deps that did parse.
func Scan(root string) ([]Dep, []error) {
	var deps []Dep
	var errs []error
	base := strings.Count(filepath.Clean(root), string(filepath.Separator))
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.Count(path, string(filepath.Separator))-base >= maxDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		p := parserFor(d.Name())
		if p == nil {
			return nil
		}
		found, err := p(path)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		for i := range found {
			found[i].Project = root
			found[i].Manifest = path
		}
		deps = append(deps, found...)
		return nil
	})
	return deps, errs
}
