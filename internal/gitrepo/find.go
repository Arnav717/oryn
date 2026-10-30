package gitrepo

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Found is a git repository under a searched folder.
type Found struct {
	Dir    string
	Ref    Ref
	GitHub bool
}

const (
	findDepth = 3   // folders below the root to look through
	findLimit = 500 // stop here rather than crawl a whole disk
)

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "target": true, "dist": true, "build": true,
	"venv": true, "__pycache__": true,
}

// Find lists git repositories at or under root, GitHub ones first. It doesn't
// look inside a repository, so nested checkouts of dependencies stay out.
func Find(root string) ([]Found, error) {
	if _, err := os.ReadDir(root); err != nil {
		return nil, err
	}
	var dirs []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if len(dirs) >= findLimit {
			return
		}
		// A file named .git is a worktree or submodule, not a repo of its own.
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			dirs = append(dirs, dir)
			return
		}
		if depth == findDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // no permission: skip it, keep the rest
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !skipDirs[e.Name()] {
				walk(filepath.Join(dir, e.Name()), depth+1)
			}
		}
	}
	walk(root, 0)

	out := make([]Found, len(dirs))
	for i, d := range dirs {
		ref, ok := Detect(d)
		out[i] = Found{Dir: d, Ref: ref, GitHub: ok}
	}
	slices.SortStableFunc(out, func(a, b Found) int {
		if a.GitHub != b.GitHub {
			if a.GitHub {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(filepath.Base(a.Dir)), strings.ToLower(filepath.Base(b.Dir)))
	})
	return out, nil
}
