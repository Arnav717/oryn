package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/source"
)

func shellModel() Model {
	m := testModel("", "/code/a", "/code/b")
	m.w, m.h = 120, 30
	m.snap = &app.Snapshot{}
	m.index()
	return m
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.handleKey(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func TestNumberKeysSwitchTabsFromAnywhere(t *testing.T) {
	want := []screen{screenHome, screenProjects, screenAgents, screenDeploys, screenGitHub, screenHealth, screenChanges}
	start := map[string]Model{}
	start["home"] = shellModel()
	inProject := shellModel()
	inProject.openScope("/code/a")
	start["project"] = inProject
	onHealthTab := inProject
	onHealthTab.tab = tabHealth
	start["project health"] = onHealthTab
	withDetail := shellModel()
	withDetail.openSection(secHealth)
	withDetail.detail = "npm/x"
	start["package detail"] = withDetail

	for name, m := range start {
		for i, s := range want {
			got := press(t, m, string(rune('1'+i)))
			if got.screen != s || got.detail != "" {
				t.Errorf("%s: key %d -> screen %d detail %q, want screen %d", name, i+1, got.screen, got.detail, s)
			}
			if got.section() != sectionID(i) {
				t.Errorf("%s: key %d highlights tab %d", name, i+1, got.section())
			}
		}
	}
}

func TestNumberKeysStayInInputs(t *testing.T) {
	m := shellModel()
	m.screen = screenProjects
	next, _ := m.startAdd("/code/")
	m = press(t, next.(Model), "3")
	if m.screen != screenProjects || m.pathInput.Value() != "/code/3" {
		t.Errorf("typing a path: screen %d value %q", m.screen, m.pathInput.Value())
	}

	m = shellModel()
	m.screen, m.runBack = screenDeployRun, screenDeploys
	m.run = &deployRun{cancel: func() {}}
	if got := press(t, m, "1"); got.screen != screenDeployRun {
		t.Error("a number key left the deploy progress screen")
	}

	m = shellModel()
	m.screen, m.pickBack = screenPicker, screenDeploys
	if got := press(t, m, "1"); got.screen != screenPicker {
		t.Error("a number key left the link picker")
	}
}

func TestEscGoesBack(t *testing.T) {
	m := shellModel()
	if got := press(t, m, "esc"); got.screen != screenHome {
		t.Errorf("esc on home -> %d, must not quit or move", got.screen)
	}
	m.openSection(secProjects)
	m.openScope("/code/a")
	for tab := projTab(0); tab < numTabs; tab++ {
		m.tab = tab
		if got := press(t, m, "esc"); got.screen != screenProjects {
			t.Errorf("esc on project tab %d -> %d, want the project list", tab, got.screen)
		}
	}
	if got := press(t, shellModel(), "2", "esc"); got.screen != screenHome {
		t.Errorf("esc on projects -> %d", got.screen)
	}
	if got := press(t, shellModel(), "6", "esc"); got.screen != screenHome {
		t.Errorf("esc on health -> %d", got.screen)
	}
}

func TestProjectOpensOnGitHubNotDependencies(t *testing.T) {
	m := shellModel()
	m.openScope("/code/a")
	if m.tab != tabGitHub {
		t.Errorf("project opened on tab %d", m.tab)
	}
}

func TestHealthTabCoversAllProjects(t *testing.T) {
	m := shellModel()
	m.openScope("/code/a")
	got := press(t, m, "6")
	if got.scope != "" || got.tabBarVisible() {
		t.Errorf("health tab: scope %q tabBar %v", got.scope, got.tabBarVisible())
	}
}

func TestAccountsReturnsWhereItOpened(t *testing.T) {
	m := shellModel()
	m.openScope("/code/a")
	m.tab = tabDeploys
	got := press(t, m, "A")
	if got.screen != screenAccounts || got.section() != secProjects {
		t.Fatalf("accounts: screen %d section %d", got.screen, got.section())
	}
	if got = press(t, got, "esc"); got.screen != screenProject {
		t.Errorf("esc from accounts -> %d, want the project", got.screen)
	}
}

func TestHomeLeadsWithWorkNotVulns(t *testing.T) {
	m := shellModel()
	m.snap.Vulns = map[string][]source.Vuln{"x": {{ID: "CVE-1", Severity: "CRITICAL", Summary: "boom", Package: "lodash"}}}
	m.index()
	out := strings.Join(m.homeLines(), "\n")
	if strings.Contains(out, "lodash") || strings.Contains(out, "CVE-1") {
		t.Errorf("home lists individual vulnerabilities:\n%s", out)
	}
	if strings.Index(out, "PROJECTS") > strings.Index(out, "HEALTH") {
		t.Errorf("health should come last:\n%s", out)
	}
}

func TestEveryScreenFitsTheTerminal(t *testing.T) {
	for _, sz := range [][2]int{{60, 14}, {80, 24}, {120, 30}, {220, 50}} {
		m := shellModel()
		m.w, m.h = sz[0], sz[1]
		check := func(name string, m Model) {
			t.Helper()
			lines := strings.Split(m.View(), "\n")
			if len(lines) > m.h {
				t.Errorf("%dx%d %s: %d lines tall", m.w, m.h, name, len(lines))
			}
			for _, l := range lines {
				if w := lipgloss.Width(l); w > m.w {
					t.Errorf("%dx%d %s: line is %d wide: %q", m.w, m.h, name, w, l)
				}
			}
		}
		for id := range numSections {
			m := m
			m.openSection(id)
			check(sections[id].title, m)
		}
		m.openScope("/code/a")
		for tab := projTab(0); tab < numTabs; tab++ {
			m.tab = tab
			check(tabNames[tab], m)
		}
	}
}

func TestNarrowTabBarKeepsActiveTitle(t *testing.T) {
	m := shellModel()
	m.w = 60
	m.openSection(secDeploys)
	line := m.tabsLine()
	if !strings.Contains(line, "Deploys") || strings.Contains(line, "Projects") {
		t.Errorf("narrow tab bar: %q", line)
	}
	if !strings.Contains(line, "7") {
		t.Errorf("narrow tab bar lost a tab number: %q", line)
	}
}
