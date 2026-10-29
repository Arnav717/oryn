package detect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// nodeManifest holds the direct dependency names declared in the package.json
// next to a lockfile. yarn.lock has no notion of "direct", so we need it.
type nodeManifest struct{ prod, dev map[string]bool }

func readPackageJSON(dir string) nodeManifest {
	m := nodeManifest{prod: map[string]bool{}, dev: map[string]bool{}}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return m
	}
	var pj struct {
		Dependencies         map[string]string
		DevDependencies      map[string]string
		OptionalDependencies map[string]string
	}
	if json.Unmarshal(b, &pj) != nil {
		return m
	}
	for n := range pj.Dependencies {
		m.prod[n] = true
	}
	for n := range pj.OptionalDependencies {
		m.prod[n] = true
	}
	for n := range pj.DevDependencies {
		m.dev[n] = true
	}
	return m
}

func (m nodeManifest) direct(name string) bool  { return m.prod[name] || m.dev[name] }
func (m nodeManifest) devOnly(name string) bool { return m.dev[name] && !m.prod[name] }

// parseYarnLock handles yarn classic (v1) and berry (v2+) lockfiles.
func parseYarnLock(path string) ([]Dep, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	man := readPackageJSON(filepath.Dir(path))
	var deps []Dep
	seen := map[string]bool{}
	name := "" // package of the entry being read; "" while skipping
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "" || line[0] == '#':
		case line[0] != ' ':
			name = yarnEntryName(line)
		case name != "" && strings.HasPrefix(line, "  version"):
			v := strings.Trim(strings.TrimPrefix(strings.TrimSpace(line), "version"), `:" `)
			if k := name + "@" + v; v != "" && !seen[k] {
				seen[k] = true
				deps = append(deps, Dep{Ecosystem: NPM, Name: name, Version: v, Direct: man.direct(name), Dev: man.devOnly(name)})
			}
			name = ""
		}
	}
	return deps, sc.Err()
}

// yarnEntryName extracts the package name from an entry header such as
// `"@babel/core@^7.0.0", "@babel/core@^7.1.0":` or `"lodash@npm:^4":`.
// Workspace entries and berry's __metadata return "".
func yarnEntryName(header string) string {
	h := strings.TrimSuffix(strings.TrimSpace(header), ":")
	first := strings.Trim(strings.TrimSpace(strings.Split(h, ",")[0]), `"`)
	if len(first) < 2 {
		return ""
	}
	i := strings.Index(first[1:], "@") // skip the scope's leading @
	if i < 0 {
		return ""
	}
	i++
	if strings.HasPrefix(first[i+1:], "workspace:") {
		return ""
	}
	return first[:i]
}

type pnpmRef string

// UnmarshalYAML accepts both `lodash: 4.17.21` (v5) and
// `lodash: {specifier: ^4, version: 4.17.21}` (v6+).
func (r *pnpmRef) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*r = pnpmRef(n.Value)
		return nil
	}
	var v struct {
		Version string `yaml:"version"`
	}
	if err := n.Decode(&v); err != nil {
		return err
	}
	*r = pnpmRef(v.Version)
	return nil
}

type pnpmImporter struct {
	Dependencies         map[string]pnpmRef `yaml:"dependencies"`
	DevDependencies      map[string]pnpmRef `yaml:"devDependencies"`
	OptionalDependencies map[string]pnpmRef `yaml:"optionalDependencies"`
}

// parsePNPMLock handles pnpm-lock.yaml v5 through v9.
func parsePNPMLock(path string) ([]Dep, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock struct {
		LockfileVersion      any                     `yaml:"lockfileVersion"`
		Dependencies         map[string]pnpmRef      `yaml:"dependencies"`
		DevDependencies      map[string]pnpmRef      `yaml:"devDependencies"`
		OptionalDependencies map[string]pnpmRef      `yaml:"optionalDependencies"`
		Importers            map[string]pnpmImporter `yaml:"importers"`
		Packages             map[string]struct {
			Dev *bool `yaml:"dev"`
		} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	importers := lock.Importers
	if len(importers) == 0 { // v5/v6 single-project layout
		importers = map[string]pnpmImporter{".": {lock.Dependencies, lock.DevDependencies, lock.OptionalDependencies}}
	}

	direct := map[string]bool{} // name@version -> dev only
	mark := func(refs map[string]pnpmRef, dev bool) {
		for name, ref := range refs {
			v := pnpmVersion(string(ref))
			if v == "" {
				continue
			}
			k := name + "@" + v
			if prev, ok := direct[k]; ok {
				dev = dev && prev
			}
			direct[k] = dev
		}
	}
	for _, imp := range importers {
		mark(imp.Dependencies, false)
		mark(imp.OptionalDependencies, false)
		mark(imp.DevDependencies, true)
	}

	v5 := strings.HasPrefix(fmt.Sprint(lock.LockfileVersion), "5")
	var deps []Dep
	for key, p := range lock.Packages {
		name, ver := pnpmPackage(key, v5)
		if name == "" {
			continue
		}
		dev, isDirect := direct[name+"@"+ver]
		if p.Dev != nil {
			dev = *p.Dev
		}
		deps = append(deps, Dep{Ecosystem: NPM, Name: name, Version: ver, Direct: isDirect, Dev: dev})
	}
	return deps, nil
}

// pnpmVersion strips peer-dependency suffixes: 1.0.0(react@18) and 1.0.0_react@18.
func pnpmVersion(v string) string {
	if i := strings.IndexAny(v, "(_"); i >= 0 {
		v = v[:i]
	}
	if v == "" || v[0] < '0' || v[0] > '9' {
		return "" // link:, file:, git URLs
	}
	return v
}

// pnpmPackage splits a `packages:` key into name and version.
func pnpmPackage(key string, v5 bool) (string, string) {
	key = strings.TrimPrefix(key, "/")
	if v5 { // /name/1.2.3_peer
		i := strings.LastIndex(key, "/")
		if i <= 0 {
			return "", ""
		}
		v := pnpmVersion(key[i+1:])
		if v == "" {
			return "", ""
		}
		return key[:i], v
	}
	if i := strings.Index(key, "("); i >= 0 { // name@1.2.3(peer@1)
		key = key[:i]
	}
	i := strings.LastIndex(key, "@")
	if i <= 0 {
		return "", ""
	}
	v := pnpmVersion(key[i+1:])
	if v == "" {
		return "", ""
	}
	return key[:i], v
}
