package tui

import (
	"testing"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/detect"
	"github.com/Arnav717/oryn/internal/source"
)

func testModel(cwd string, projects ...string) Model {
	m := New(&config.Config{Projects: projects, Path: ""}, nil)
	m.cwd = cwd
	m.homeCursor = m.cwdItem()
	return m
}

func TestHomeItems(t *testing.T) {
	m := testModel("/code/other", "/code/a", "/code/b")
	kinds := []itemKind{itemProject, itemProject, itemAddCwd, itemAddPath}
	items := m.homeItems()
	if len(items) != len(kinds) {
		t.Fatalf("got %d items: %+v", len(items), items)
	}
	for i, k := range kinds {
		if items[i].kind != k {
			t.Errorf("item %d: kind %v, want %v", i, items[i].kind, k)
		}
	}

	// Inside a watched project: no add-cwd row.
	m = testModel("/code/a/sub", "/code/a")
	items = m.homeItems()
	if len(items) != 2 || items[0].kind != itemProject || items[1].kind != itemAddPath {
		t.Errorf("single project inside cwd: %+v", items)
	}

	// Nothing watched: offer cwd first.
	m = testModel("/code/new")
	items = m.homeItems()
	if len(items) != 2 || items[0].kind != itemAddCwd || m.homeCursor != 0 {
		t.Errorf("empty: %+v cursor=%d", items, m.homeCursor)
	}
}

func TestCwdPreselect(t *testing.T) {
	// Deepest watched root wins.
	m := testModel("/code/mono/pkg/x", "/code/mono", "/code/mono/pkg", "/code/zzz")
	items := m.homeItems()
	if got := items[m.homeCursor]; got.kind != itemProject || got.path != "/code/mono/pkg" {
		t.Errorf("preselected %+v", got)
	}
}

func TestStatsScoping(t *testing.T) {
	a, b := "/code/a", "/code/b"
	dep := func(proj, name, ver string) detect.Dep {
		return detect.Dep{Ecosystem: detect.NPM, Name: name, Version: ver, Direct: true, Project: proj}
	}
	deps := []detect.Dep{dep(a, "lodash", "4.0.0"), dep(a, "react", "18.0.0"), dep(b, "lodash", "4.0.0")}
	lodash := source.DepKey(deps[0])
	m := testModel("", a, b)
	m.snap = &app.Snapshot{
		Deps:   deps,
		Vulns:  map[string][]source.Vuln{lodash: {{ID: "X", Severity: "CRITICAL", Ecosystem: "npm", Package: "lodash", Version: "4.0.0"}}},
		Latest: map[string]string{"npm/react": "19.0.0"},
	}
	m.index()

	if st := m.stats[a]; st.Deps != 2 || st.Vulns != 1 || st.Crit != 1 || st.Outdated != 1 {
		t.Errorf("a: %+v", st)
	}
	if st := m.stats[b]; st.Deps != 1 || st.Vulns != 1 || st.Outdated != 0 {
		t.Errorf("b: %+v", st)
	}
	// Same dep+version in two projects is one vulnerability overall.
	if m.total.Vulns != 1 || m.total.Deps != 3 {
		t.Errorf("total: %+v", m.total)
	}

	m.openScope(b)
	if len(m.pkgs) != 1 || len(m.vulns) != 1 {
		t.Errorf("scoped to b: %d pkgs, %d vulns", len(m.pkgs), len(m.vulns))
	}
	m.openScope("")
	if len(m.pkgs) != 2 {
		t.Errorf("all projects: %d pkgs, want 2", len(m.pkgs))
	}
}
