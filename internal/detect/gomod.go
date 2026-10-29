package detect

import (
	"os"

	"golang.org/x/mod/modfile"
)

func parseGoMod(path string) ([]Dep, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := modfile.ParseLax(path, b, nil)
	if err != nil {
		return nil, err
	}
	deps := make([]Dep, 0, len(f.Require))
	for _, r := range f.Require {
		deps = append(deps, Dep{
			Ecosystem: Go,
			Name:      r.Mod.Path,
			Version:   r.Mod.Version,
			Direct:    !r.Indirect,
		})
	}
	return deps, nil
}
