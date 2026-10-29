package detect

import (
	"bufio"
	"os"
	"strings"
)

// parseRequirements handles requirements*.txt. Only `name==version` pins carry
// a version; unpinned requirements are kept with an empty version.
func parseRequirements(path string) ([]Dep, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var deps []Dep
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i] // environment marker
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") || strings.Contains(line, "://") {
			continue // options, includes, URLs
		}
		name, version := line, ""
		if i := strings.Index(line, "=="); i >= 0 {
			name, version = line[:i], strings.TrimSpace(line[i+2:])
		} else if i := strings.IndexAny(line, "<>=!~ "); i >= 0 {
			name = line[:i]
		}
		if i := strings.Index(name, "["); i >= 0 {
			name = name[:i] // extras
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		deps = append(deps, Dep{Ecosystem: PyPI, Name: name, Version: version, Direct: true})
	}
	return deps, sc.Err()
}
