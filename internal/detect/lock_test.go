package detect

import "testing"

func byName(deps []Dep) map[string]Dep {
	m := map[string]Dep{}
	for _, d := range deps {
		m[d.Name+"@"+d.Version] = d
	}
	return m
}

func expect(t *testing.T, got map[string]Dep, key string, direct, dev bool) {
	t.Helper()
	d, ok := got[key]
	if !ok {
		t.Errorf("missing %s in %v", key, got)
		return
	}
	if d.Direct != direct || d.Dev != dev {
		t.Errorf("%s: direct=%v dev=%v, want direct=%v dev=%v", key, d.Direct, d.Dev, direct, dev)
	}
}

func TestParseYarnV1(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"dependencies":{"lodash":"^4"},"devDependencies":{"@babel/code-frame":"^7"}}`)
	p := write(t, dir, "yarn.lock", `# yarn lockfile v1


"@babel/code-frame@^7.0.0", "@babel/code-frame@^7.10.4":
  version "7.12.13"
  resolved "https://registry.yarnpkg.com/x"
  dependencies:
    "@babel/highlight" "^7.10.4"

lodash@^4.17.21:
  version "4.17.21"

"@babel/highlight@^7.10.4":
  version "7.10.4"
`)
	deps, err := parseYarnLock(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(deps)
	if len(got) != 3 {
		t.Fatalf("want 3 deps, got %v", got)
	}
	expect(t, got, "@babel/code-frame@7.12.13", true, true)
	expect(t, got, "lodash@4.17.21", true, false)
	expect(t, got, "@babel/highlight@7.10.4", false, false)
}

func TestParseYarnBerry(t *testing.T) {
	p := write(t, t.TempDir(), "yarn.lock", `__metadata:
  version: 6

"lodash@npm:^4.17.21":
  version: 4.17.21
  resolution: "lodash@npm:4.17.21"

"app@workspace:.":
  version: 0.0.0-use.local
`)
	deps, err := parseYarnLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Name != "lodash" || deps[0].Version != "4.17.21" {
		t.Fatalf("got %+v", deps)
	}
}

func TestParsePNPMv9(t *testing.T) {
	p := write(t, t.TempDir(), "pnpm-lock.yaml", `lockfileVersion: '9.0'
importers:
  .:
    dependencies:
      lodash:
        specifier: ^4.17.0
        version: 4.17.21
    devDependencies:
      vitest:
        specifier: ^1
        version: 1.0.0(@types/node@20.0.0)
packages:
  lodash@4.17.21:
    resolution: {integrity: sha512-x}
  vitest@1.0.0:
    resolution: {integrity: sha512-y}
  '@babel/core@7.0.0':
    resolution: {integrity: sha512-z}
`)
	deps, err := parsePNPMLock(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(deps)
	if len(got) != 3 {
		t.Fatalf("want 3 deps, got %v", got)
	}
	expect(t, got, "lodash@4.17.21", true, false)
	expect(t, got, "vitest@1.0.0", true, true)
	expect(t, got, "@babel/core@7.0.0", false, false)
}

func TestParsePNPMv5(t *testing.T) {
	p := write(t, t.TempDir(), "pnpm-lock.yaml", `lockfileVersion: 5.4
specifiers:
  lodash: ^4
dependencies:
  lodash: 4.17.21
devDependencies:
  jest: 27.0.0_peer@1
packages:
  /lodash/4.17.21:
    dev: false
  /jest/27.0.0_peer@1:
    dev: true
  /@babel/core/7.0.0:
    dev: true
`)
	deps, err := parsePNPMLock(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(deps)
	expect(t, got, "lodash@4.17.21", true, false)
	expect(t, got, "jest@27.0.0", true, true)
	expect(t, got, "@babel/core@7.0.0", false, true)
}

func TestParseCargoLock(t *testing.T) {
	p := write(t, t.TempDir(), "Cargo.lock", `version = 3

[[package]]
name = "myapp"
version = "0.1.0"
dependencies = [
 "serde",
 "tokio 1.0.0 (registry+https://github.com/rust-lang/crates.io-index)",
]

[[package]]
name = "serde"
version = "1.0.100"
source = "registry+https://github.com/rust-lang/crates.io-index"
dependencies = ["serde_derive"]

[[package]]
name = "serde_derive"
version = "1.0.100"
source = "registry+https://github.com/rust-lang/crates.io-index"

[[package]]
name = "tokio"
version = "1.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"

[[package]]
name = "fromgit"
version = "0.1.0"
source = "git+https://github.com/x/y#abc"
`)
	deps, err := parseCargoLock(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(deps)
	if len(got) != 3 {
		t.Fatalf("want 3 deps (no member, no git), got %v", got)
	}
	expect(t, got, "serde@1.0.100", true, false)
	expect(t, got, "tokio@1.0.0", true, false)
	expect(t, got, "serde_derive@1.0.100", false, false)
	if got["serde@1.0.100"].Ecosystem != Crates {
		t.Errorf("ecosystem = %q", got["serde@1.0.100"].Ecosystem)
	}
}

func TestParseUVLock(t *testing.T) {
	p := write(t, t.TempDir(), "uv.lock", `version = 1

[[package]]
name = "myapp"
version = "0.1.0"
source = { editable = "." }
dependencies = [{ name = "requests" }]

[package.dev-dependencies]
dev = [{ name = "pytest" }]

[[package]]
name = "requests"
version = "2.31.0"
source = { registry = "https://pypi.org/simple" }
dependencies = [{ name = "urllib3" }]

[[package]]
name = "urllib3"
version = "2.0.0"
source = { registry = "https://pypi.org/simple" }

[[package]]
name = "pytest"
version = "8.0.0"
source = { registry = "https://pypi.org/simple" }
`)
	deps, err := parseUVLock(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(deps)
	if len(got) != 3 {
		t.Fatalf("want 3 deps, got %v", got)
	}
	expect(t, got, "requests@2.31.0", true, false)
	expect(t, got, "pytest@8.0.0", true, true)
	expect(t, got, "urllib3@2.0.0", false, false)
}

func TestParsePoetryLock(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pyproject.toml", `[tool.poetry.dependencies]
python = "^3.11"
django = "^4.2"

[tool.poetry.group.dev.dependencies]
pytest = "^8"
`)
	p := write(t, dir, "poetry.lock", `[[package]]
name = "Django"
version = "4.2.0"

[[package]]
name = "sqlparse"
version = "0.4.0"

[[package]]
name = "pytest"
version = "8.0.0"
`)
	deps, err := parsePoetryLock(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(deps)
	expect(t, got, "django@4.2.0", true, false)
	expect(t, got, "sqlparse@0.4.0", false, false)
	expect(t, got, "pytest@8.0.0", true, true)
}

func TestNormPyPI(t *testing.T) {
	for in, want := range map[string]string{"Django": "django", "typing_extensions": "typing-extensions", "zope.interface": "zope-interface"} {
		if got := normPyPI(in); got != want {
			t.Errorf("normPyPI(%q) = %q, want %q", in, got, want)
		}
	}
}
