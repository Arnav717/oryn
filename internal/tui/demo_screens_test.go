package tui

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Arnav717/oryn/internal/agents"
	"github.com/Arnav717/oryn/internal/app"
	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/deploy"
	"github.com/Arnav717/oryn/internal/detect"
	"github.com/Arnav717/oryn/internal/gitrepo"
	"github.com/Arnav717/oryn/internal/source"
	"github.com/Arnav717/oryn/internal/store"
)

// demoModel is a made-up workspace for screenshots, so none of a maintainer's
// real projects, people or tokens ever appear in the README.
func demoModel() Model {
	now := time.Date(2026, 10, 30, 9, 41, 0, 0, time.Local)
	root := "/home/dev/code/"
	names := []string{"storefront", "api-gateway", "mobile-app", "docs-site", "ml-pipeline"}
	var projects []string
	for _, n := range names {
		projects = append(projects, root+n)
	}
	cfg := &config.Config{Projects: projects}
	m := New(cfg, nil)
	m.cwd, m.now, m.w, m.h = root+"storefront", now, 150, 30
	m.loading = false
	ago := func(d time.Duration) time.Time { return now.Add(-d) }

	repo := func(n string, ci string, prs, issues int, stars int) *source.RepoData {
		d := &source.RepoData{Ref: gitrepo.Ref{Owner: "acme", Name: n}, Viewer: "dev", URL: "https://github.com/acme/" + n,
			DefaultBranch: "main", DefaultCI: ci, OpenPRs: prs, OpenIssues: issues, Stars: stars, Forks: stars / 6, Language: "TypeScript", License: "MIT",
			Description: "Part of the Acme platform", AlertsKnown: true}
		d.Commits = []source.Commit{
			{Message: "Add retry with backoff to the payments client", Author: "maya", At: ago(55 * time.Minute)},
			{Message: "Fix cart total rounding for multi-currency orders", Author: "dev", At: ago(3 * time.Hour)},
			{Message: "Bump Next.js to 15.1.2", Author: "dependabot", At: ago(26 * time.Hour)},
		}
		d.Runs = []source.Run{{Number: 412, Workflow: "ci", Branch: "main", Status: "completed", Conclusion: map[string]string{"FAILURE": "failure"}[ci], Created: ago(2 * time.Hour)}}
		return d
	}
	byProject := map[string]*source.RepoData{
		root + "storefront":  repo("storefront", "FAILURE", 6, 14, 482),
		root + "api-gateway": repo("api-gateway", "SUCCESS", 3, 9, 211),
		root + "mobile-app":  repo("mobile-app", "SUCCESS", 2, 21, 97),
		root + "docs-site":   repo("docs-site", "SUCCESS", 0, 3, 40),
		root + "ml-pipeline": repo("ml-pipeline", "SUCCESS", 1, 6, 156),
	}
	byProject[root+"storefront"].Issues = []source.Issue{{Number: 231, Title: "Checkout hangs on 3DS redirect", Assignees: []string{"dev"}, Updated: ago(5 * time.Hour)}}
	refs := map[string]gitrepo.Ref{}
	for p, d := range byProject {
		refs[p] = d.Ref
	}

	dep := func(p, name, ver string, direct bool) detect.Dep {
		return detect.Dep{Ecosystem: "npm", Name: name, Version: ver, Direct: direct, Project: root + p}
	}
	deps := []detect.Dep{
		dep("storefront", "next", "15.1.2", true), dep("storefront", "react", "19.0.0", true), dep("storefront", "stripe", "16.2.0", true),
		dep("storefront", "zod", "3.23.8", true), dep("storefront", "lodash", "4.17.20", true), dep("storefront", "axios", "1.6.0", true),
		dep("api-gateway", "express", "4.18.1", true), dep("api-gateway", "fastify", "5.1.0", true), dep("api-gateway", "pino", "9.4.0", true),
		dep("api-gateway", "jsonwebtoken", "9.0.0", true), dep("api-gateway", "undici", "6.19.2", false),
		dep("mobile-app", "expo", "52.0.4", true), dep("mobile-app", "react-native", "0.76.1", true), dep("mobile-app", "zustand", "5.0.0", true),
		dep("docs-site", "astro", "4.16.0", true), dep("docs-site", "tailwindcss", "3.4.14", true),
	}
	vuln := func(d detect.Dep, id, sev, sum, fix string) source.Vuln {
		return source.Vuln{ID: id, Aliases: []string{id}, Severity: sev, Summary: sum, Fixed: fix, Ecosystem: "npm", Package: d.Name, Version: d.Version}
	}
	vulns := map[string][]source.Vuln{
		source.DepKey(deps[0]): {vuln(deps[0], "CVE-2025-29927", "CRITICAL", "Authorization bypass in middleware", "15.2.3"), vuln(deps[0], "GHSA-7gfc-8cq8-jh5f", "HIGH", "Cache poisoning through crafted headers", "15.1.6")},
		source.DepKey(deps[4]): {vuln(deps[4], "CVE-2025-31811", "HIGH", "Prototype pollution in zipObjectDeep", "4.17.21")},
		source.DepKey(deps[5]): {vuln(deps[5], "CVE-2025-27152", "MEDIUM", "Server-side request forgery via absolute URLs", "1.8.2")},
		source.DepKey(deps[6]): {vuln(deps[6], "CVE-2024-45590", "HIGH", "Denial of service in body-parser", "4.19.2")},
		source.DepKey(deps[9]): {vuln(deps[9], "CVE-2025-10901", "MEDIUM", "Insecure key handling in verify", "9.0.2")},
	}
	latest := map[string]string{"npm/next": "15.3.1", "npm/react": "19.1.0", "npm/stripe": "17.4.0", "npm/zod": "3.24.2", "npm/lodash": "4.17.21", "npm/axios": "1.8.4",
		"npm/express": "4.21.2", "npm/fastify": "5.3.0", "npm/pino": "9.6.0", "npm/jsonwebtoken": "9.0.2", "npm/expo": "52.0.18", "npm/react-native": "0.79.1", "npm/zustand": "5.0.3", "npm/astro": "5.5.4", "npm/tailwindcss": "4.1.0"}

	dp := func(project, platform, url string, deps ...deploy.Deployment) app.DeployTarget {
		return app.DeployTarget{Link: deploy.Link{Platform: platform, Project: root + project, Ref: "x"}, Queryable: true, Deployments: deps, LiveURL: url}
	}
	prod := func(state string, age time.Duration) deploy.Deployment {
		return deploy.Deployment{ID: state + age.String(), Target: "production", State: state, Branch: "main", Commit: "9f3c2ab", Message: "Release", Created: ago(age)}
	}
	snap := &app.Snapshot{
		At: now, Deps: deps, Vulns: vulns, Latest: latest, RepoRefs: refs, Repos: byProject,
		Deploys: []app.DeployTarget{
			dp("storefront", "vercel", "https://storefront.acme.dev", prod("error", 95*time.Minute), prod("ready", 30*time.Hour)),
			dp("docs-site", "cloudflare", "https://docs.acme.dev", prod("ready", 4*time.Hour)),
			dp("api-gateway", "vercel", "https://api.acme.dev", prod("ready", 26*time.Hour)),
			dp("mobile-app", "netlify", "https://app.acme.dev", prod("ready", 3*24*time.Hour)),
		},
		GitHub: &source.GitHubData{Login: "dev",
			Review: []source.PR{
				{Repo: "acme/api-gateway", Number: 88, Title: "Add request tracing with OpenTelemetry", Author: "maya", Updated: ago(2 * time.Hour), CI: "SUCCESS", Review: "REVIEW_REQUIRED"},
				{Repo: "acme/mobile-app", Number: 143, Title: "Offline mode for the order history screen", Author: "jordan", Updated: ago(7 * time.Hour), CI: "PENDING"},
				{Repo: "acme/ml-pipeline", Number: 61, Title: "Cache feature embeddings between runs", Author: "sam", Updated: ago(26 * time.Hour), CI: "SUCCESS"},
			},
			Mine: []source.PR{{Repo: "acme/storefront", Number: 512, Title: "Fix 3DS redirect loop on checkout", Author: "dev", Updated: ago(40 * time.Minute), CI: "FAILURE", Review: "CHANGES_REQUESTED"}},
		},
		Status: []source.Status{
			{Service: "GitHub", Indicator: "none", Description: "All Systems Operational"}, {Service: "npm", Indicator: "none", Description: "All Systems Operational"},
			{Service: "Vercel", Indicator: "none", Description: "All Systems Operational"}, {Service: "Cloudflare", Indicator: "minor", Description: "Degraded performance in EU"},
			{Service: "Stripe", Indicator: "none", Description: "All Systems Operational", Via: "stripe"}, {Service: "Claude", Indicator: "none", Description: "All Systems Operational"},
		},
		Events: []store.Event{
			{Key: "e1", Kind: "deploy", Severity: 4, Title: "vercel deploy failed (production main)", Entity: root + "storefront", At: ago(95 * time.Minute)},
			{Key: "e2", Kind: "ci", Severity: 3, Title: "CI failing on acme/storefront main", Entity: "acme/storefront", At: ago(2 * time.Hour)},
			{Key: "e3", Kind: "review", Severity: 1, Title: "review requested: acme/api-gateway#88 Add request tracing (maya)", Entity: "acme/api-gateway", At: ago(2 * time.Hour)},
			{Key: "e4", Kind: "vuln", Severity: 4, Title: "CVE-2025-29927 in next@15.1.2, fixed in 15.2.3", Entity: "npm/next", At: ago(5 * time.Hour)},
			{Key: "e5", Kind: "status", Severity: 2, Title: "Cloudflare: Degraded performance in EU", Entity: "Cloudflare", At: ago(6 * time.Hour)},
			{Key: "e6", Kind: "release", Severity: 0, Title: "react 19.1.0 available (you have 19.0.0)", Entity: "npm/react", At: ago(20 * time.Hour)},
			{Key: "e7", Kind: "pr", Severity: 1, Title: "acme/mobile-app#143 Offline mode for the order history screen", Entity: "acme/mobile-app", At: ago(7 * time.Hour)},
		},
	}
	m.snap = snap
	m.since = ago(9 * time.Hour)
	m.index()
	m.agents = []agents.Session{
		{Tool: "claude", ID: "a", Dir: "/s/wt/storefront/fix-3ds", Project: root + "storefront", Title: "Fix the 3DS redirect loop on checkout", State: agents.Working, Updated: ago(20 * time.Second),
			Run: &agents.Run{Slug: "fix-3ds", Branch: "oryn/fix-3ds-redirect-loop", Task: "Fix the 3DS redirect loop on checkout", Project: root + "storefront"}, Running: true},
		{Tool: "codex", ID: "b", Dir: root + "api-gateway", Title: "Add rate limiting per API key", State: agents.Waiting, Needs: "Approve a command: press a to answer", Updated: ago(3 * time.Minute)},
		{Tool: "opencode", ID: "c", Dir: root + "mobile-app", Title: "Migrate navigation to the new router", State: agents.Working, Updated: ago(40 * time.Second)},
		{Tool: "agy", ID: "d", Dir: root + "ml-pipeline", Title: "Profile and speed up the embedding step", State: agents.Idle, Updated: ago(52 * time.Minute)},
		{Tool: "claude", ID: "e", Dir: root + "docs-site", Title: "Write the migration guide for v3", State: agents.Done, Updated: ago(5 * time.Hour)},
		{Tool: "claude", ID: "f", Dir: root + "api-gateway", Title: "Upgrade express and fix the body-parser advisory", State: agents.Done, Updated: ago(27 * time.Hour)},
	}
	sort.SliceStable(m.agents, func(i, j int) bool {
		if m.agents[i].State != m.agents[j].State {
			return m.agents[i].State < m.agents[j].State
		}
		return m.agents[i].Updated.After(m.agents[j].Updated)
	})
	return m
}

// TestDemoScreens writes each demo screen, with colour codes, into $DEMO_OUT.
// `make screenshots` renders them to images.
func TestDemoScreens(t *testing.T) {
	out := os.Getenv("DEMO_OUT")
	if out == "" {
		t.Skip()
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	save := func(name string, m Model) {
		if h, ok := map[string]int{"agents": 20, "review": 30, "project": 18, "palette": 33, "health": 33, "activity": 22}[name]; ok {
			m.h = h
		}
		if err := os.WriteFile(filepath.Join(out, name+".ansi"), []byte(m.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := demoModel()
	save("home", m)

	a := m
	a.openSection(secAgents)
	a.agCursor = 1
	save("agents", a)

	r := m
	r.openSection(secAgents)
	r.agCursor = 0
	run := agents.Run{Slug: "fix-3ds", Branch: "oryn/fix-3ds-redirect-loop", Task: "Fix the 3DS redirect loop on checkout"}
	r.review = &reviewView{run: run, summary: "2 commit(s)", lines: diffLines(agents.Changes{
		Commits: 2,
		Diff: `diff --git a/src/checkout/threeDS.ts b/src/checkout/threeDS.ts
--- a/src/checkout/threeDS.ts
+++ b/src/checkout/threeDS.ts
@@ -41,10 +41,14 @@ export async function confirmPayment(intent: PaymentIntent) {
   const result = await stripe.confirmCardPayment(intent.client_secret)
-  if (result.status === 'requires_action') {
-    return redirectTo3DS(result.next_action.redirect_to_url.url)
-  }
+  if (result.status === 'requires_action') {
+    // Return to the same order, or the redirect lands on a fresh cart and loops.
+    const returnUrl = ` + "`${location.origin}/checkout/return?order=${intent.metadata.orderId}`" + `
+    return redirectTo3DS(result.next_action.redirect_to_url.url, returnUrl)
+  }
   return result
 }
diff --git a/src/checkout/threeDS.test.ts b/src/checkout/threeDS.test.ts
--- a/src/checkout/threeDS.test.ts
+++ b/src/checkout/threeDS.test.ts
@@ -12,3 +12,9 @@ describe('confirmPayment', () => {
+  it('returns to the same order after 3DS', async () => {
+    const result = await confirmPayment(requiresAction)
+    expect(redirect).toHaveBeenCalledWith(expect.any(String), expect.stringContaining('order=ord_123'))
+  })
`}), loading: false}
	save("review", r)

	p := m
	p.openScope(p.cfg.Projects[0])
	save("project", p)

	pal := m
	next, _ := pal.openPalette()
	save("palette", next.(Model))

	h := m
	h.openSection(secHealth)
	save("health", h)

	ac := m
	ac.openSection(secActivity)
	save("activity", ac)
}
