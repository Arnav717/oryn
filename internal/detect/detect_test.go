package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseNPMLock(t *testing.T) {
	p := write(t, t.TempDir(), "package-lock.json", `{
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"dependencies": {"lodash": "^4"}, "devDependencies": {"vitest": "^4"}},
	    "node_modules/lodash": {"version": "4.17.15"},
	    "node_modules/vitest": {"version": "4.1.4", "dev": true},
	    "node_modules/vitest/node_modules/lodash": {"version": "4.17.21", "dev": true},
	    "packages/app": {"version": "1.0.0"},
	    "node_modules/app": {"link": true}
	  }
	}`)
	deps, err := parseNPMLock(p)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Dep{}
	for _, d := range deps {
		got[d.Name+"@"+d.Version] = d
	}
	if len(got) != 3 {
		t.Fatalf("want 3 deps, got %v", got)
	}
	if d := got["lodash@4.17.15"]; !d.Direct || d.Dev {
		t.Errorf("lodash@4.17.15 should be direct prod: %+v", d)
	}
	if d := got["vitest@4.1.4"]; !d.Direct || !d.Dev {
		t.Errorf("vitest should be direct dev: %+v", d)
	}
	if d := got["lodash@4.17.21"]; d.Direct {
		t.Errorf("nested lodash should be transitive: %+v", d)
	}
}

func TestParseRequirements(t *testing.T) {
	p := write(t, t.TempDir(), "requirements.txt", `
# comment
Django==4.2.1  # pinned
requests[socks]==2.31.0 ; python_version > "3.8"
numpy>=1.20
-r other.txt
git+https://github.com/x/y.git
`)
	deps, err := parseRequirements(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ name, ver string }{{"django", "4.2.1"}, {"requests", "2.31.0"}, {"numpy", ""}}
	if len(deps) != len(want) {
		t.Fatalf("want %d deps, got %+v", len(want), deps)
	}
	for i, w := range want {
		if deps[i].Name != w.name || deps[i].Version != w.ver {
			t.Errorf("dep %d: want %s==%s, got %s==%s", i, w.name, w.ver, deps[i].Name, deps[i].Version)
		}
	}
}

func TestScanSkipsNodeModules(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.22\n\nrequire github.com/a/b v1.2.3\nrequire github.com/c/d v0.1.0 // indirect\n")
	nm := filepath.Join(root, "node_modules", "pkg")
	os.MkdirAll(nm, 0o755)
	write(t, nm, "requirements.txt", "flask==1.0\n")

	deps, errs := Scan(root)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(deps) != 2 {
		t.Fatalf("want 2 go deps, got %+v", deps)
	}
	if !deps[0].Direct || deps[1].Direct {
		t.Errorf("direct/indirect wrong: %+v", deps)
	}
}
