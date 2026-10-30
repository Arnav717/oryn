package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".vercel/project.json", `{"projectId":"prj_1","orgId":"team_9"}`)
	write(t, root, "netlify.toml", "")
	write(t, root, "wrangler.jsonc", `{ // pages
  "name": "edge-api", "pages_build_output_dir": "dist" }`)

	got := map[string]Link{}
	for _, l := range Detect(root) {
		got[l.Platform] = l
	}
	if l := got["vercel"]; l.Ref != "prj_1" || l.Team != "team_9" || !l.Linked() {
		t.Errorf("vercel: %+v", l)
	}
	if l := got["netlify"]; l.Linked() || l.Marker != "netlify.toml" {
		t.Errorf("netlify should be detected but unlinked: %+v", l)
	}
	if got["cloudflare"].Ref != "edge-api" {
		t.Errorf("cloudflare: %+v", got["cloudflare"])
	}
	worker := t.TempDir()
	write(t, worker, "wrangler.toml", `name = "just-a-worker"`)
	if l := Detect(worker); len(l) != 1 || l[0].Linked() {
		t.Errorf("plain worker should be detected but not linked: %+v", l)
	}
	if len(Detect(t.TempDir())) != 0 {
		t.Error("empty dir should detect nothing")
	}
}

func TestVercelList(t *testing.T) {
	t.Setenv("VERCEL_TOKEN", "tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Query().Get("projectId") != "prj_1" || r.URL.Query().Get("teamId") != "team_9" {
			http.Error(w, "bad request", 400)
			return
		}
		w.Write([]byte(`{"deployments":[
		 {"uid":"d1","url":"app-abc.vercel.app","state":"READY","target":"production","created":1700000000000,
		  "meta":{"githubCommitSha":"0123456789abcdef","githubCommitMessage":"fix: thing\n\nbody","githubCommitRef":"main"}},
		 {"uid":"d2","url":"app-def.vercel.app","state":"ERROR","created":1700000100000,"meta":{}}]}`))
	}))
	defer srv.Close()
	v := &Vercel{base: srv.URL}
	got, err := v.List(context.Background(), Link{Ref: "prj_1", Team: "team_9"}, 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if d := got[0]; d.State != StateReady || d.Target != "production" || d.Commit != "0123456" || d.Message != "fix: thing" || d.Branch != "main" {
		t.Errorf("d1: %+v", d)
	}
	if d := got[1]; d.State != StateError || d.Target != "preview" {
		t.Errorf("d2: %+v", d)
	}
}

func TestNetlifyList(t *testing.T) {
	t.Setenv("NETLIFY_AUTH_TOKEN", "tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/site-1/deploys" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "bad request", 400)
			return
		}
		w.Write([]byte(`[{"id":"n1","state":"enqueued","context":"deploy-preview","branch":"feat","commit_ref":"abcdef0123","title":"PR #4","deploy_ssl_url":"https://x--site.netlify.app","created_at":"2026-10-30T10:00:00Z"}]`))
	}))
	defer srv.Close()
	n := &Netlify{base: srv.URL}
	got, err := n.List(context.Background(), Link{Ref: "site-1"}, 5)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if d := got[0]; d.State != StateQueued || d.Target != "preview" || d.Commit != "abcdef0" || d.Created.IsZero() {
		t.Errorf("n1: %+v", d)
	}
}

func TestNoCredentials(t *testing.T) {
	t.Setenv("VERCEL_TOKEN", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if _, err := NewVercel().List(context.Background(), Link{Ref: "p"}, 1); err != ErrNoCredentials {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestCloudflareList(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acc1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/v4/accounts/acc1/pages/projects/edge-api/deployments" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "bad request", 400)
			return
		}
		w.Write([]byte(`{"success":true,"result":[
		 {"id":"c1","environment":"production","url":"https://c1.edge-api.pages.dev","created_on":"2026-10-30T10:00:00Z",
		  "deployment_trigger":{"metadata":{"branch":"main","commit_hash":"deadbeef123","commit_message":"ship it"}},
		  "latest_stage":{"name":"deploy","status":"success"}},
		 {"id":"c2","environment":"preview","url":"https://c2.edge-api.pages.dev","created_on":"2026-10-30T11:00:00Z",
		  "deployment_trigger":{"metadata":{}},"latest_stage":{"name":"build","status":"failure"}}]}`))
	}))
	defer srv.Close()
	c := &Cloudflare{base: srv.URL}
	got, err := c.List(context.Background(), Link{Ref: "edge-api"}, 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if d := got[0]; d.State != StateReady || d.Target != "production" || d.Commit != "deadbee" || d.Branch != "main" {
		t.Errorf("c1: %+v", d)
	}
	if d := got[1]; d.State != StateError || d.Target != "preview" {
		t.Errorf("c2: %+v", d)
	}
}

func TestConnectStoresOnlyVerifiedTokens(t *testing.T) {
	dir := t.TempDir()
	SetCredsDir(dir)
	t.Cleanup(func() { SetCredsDir("") })
	t.Setenv("VERCEL_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, "forbidden", 403)
			return
		}
		switch r.URL.Path {
		case "/v2/user":
			w.Write([]byte(`{"user":{"username":"arnav"}}`))
		case "/v9/projects":
			w.Write([]byte(`{"projects":[{"id":"prj_1","name":"web","accountId":"team_7"},{"id":"prj_2","name":"me","accountId":"usr_1"}]}`))
		}
	}))
	defer srv.Close()
	v := &Vercel{base: srv.URL}

	if _, err := connect(context.Background(), v, "vercel", "bad"); err == nil {
		t.Fatal("bad token must be rejected")
	}
	if Source("vercel") != "" {
		t.Fatal("rejected token must not be stored")
	}

	who, err := connect(context.Background(), v, "vercel", " good\n")
	if err != nil || who != "arnav" {
		t.Fatalf("who=%q err=%v", who, err)
	}
	if Source("vercel") != "oryn" {
		t.Fatalf("source = %q", Source("vercel"))
	}
	fi, err := os.Stat(filepath.Join(dir, "credentials.toml"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("credentials file mode: %v %v", fi, err)
	}

	ps, err := v.Projects(context.Background())
	if err != nil || len(ps) != 2 || ps[0].Team != "team_7" || ps[1].Team != "usr_1" {
		t.Fatalf("projects: %+v %v", ps, err)
	}

	if err := Disconnect("vercel"); err != nil || Source("vercel") != "" {
		t.Fatalf("disconnect: %v source=%q", err, Source("vercel"))
	}
}

func TestCloudflareConnectStoresAccount(t *testing.T) {
	SetCredsDir(t.TempDir())
	t.Cleanup(func() { SetCredsDir("") })
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[{"id":"acc9","name":"My Acct"}]}`))
	}))
	defer srv.Close()
	who, err := connect(context.Background(), &Cloudflare{base: srv.URL}, "cloudflare", "tok")
	if err != nil || who != "My Acct" {
		t.Fatalf("who=%q err=%v", who, err)
	}
	if tok, acc := cloudflareCreds(); tok != "tok" || acc != "acc9" {
		t.Fatalf("creds: %q %q", tok, acc)
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{"My Portfolio_2": "my-portfolio-2", "--x--": "x", "Vontrix.dev": "vontrix-dev", "": ""} {
		if got := SuggestName(in); got != want {
			t.Errorf("SuggestName(%q) = %q, want %q", in, got, want)
		}
	}
	for _, ok := range []string{"a", "my-site", "site2"} {
		if !ValidName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "-a", "a-", "My", "a b", "a_b", strings.Repeat("a", 60)} {
		if ValidName(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestCreate(t *testing.T) {
	SetCredsDir(t.TempDir())
	t.Cleanup(func() { SetCredsDir("") })
	t.Setenv("VERCEL_TOKEN", "tok")
	t.Setenv("NETLIFY_AUTH_TOKEN", "tok")
	t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acc1")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "bad", 400)
			return
		}
		switch r.URL.Path {
		case "/v11/projects":
			w.Write([]byte(`{"id":"prj_new","name":"web","accountId":"usr_1"}`))
		case "/api/v1/sites":
			w.Write([]byte(`{"id":"site_new","name":"web"}`))
		default:
			w.Write([]byte(`{"result":{"name":"web"}}`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if p, err := (&Vercel{base: srv.URL}).Create(ctx, "web"); err != nil || p.Ref != "prj_new" || p.Team != "usr_1" {
		t.Errorf("vercel: %+v %v", p, err)
	}
	if p, err := (&Netlify{base: srv.URL}).Create(ctx, "web"); err != nil || p.Ref != "site_new" {
		t.Errorf("netlify: %+v %v", p, err)
	}
	if p, err := (&Cloudflare{base: srv.URL}).Create(ctx, "web"); err != nil || p.Ref != "web" {
		t.Errorf("cloudflare: %+v %v", p, err)
	}
	want := "POST /v11/projects,POST /api/v1/sites,POST /client/v4/accounts/acc1/pages/projects"
	if got := strings.Join(seen, ","); got != want {
		t.Errorf("requests: %s", got)
	}
	if CreatorFor("vercel") == nil || CreatorFor("nope") != nil {
		t.Error("CreatorFor")
	}
}

func TestDeployCommand(t *testing.T) {
	SetCredsDir(t.TempDir())
	t.Cleanup(func() { SetCredsDir("") })
	t.Setenv("VERCEL_TOKEN", "secret-token")
	dir := t.TempDir()

	c, err := DeployCommand(Link{Platform: "vercel", Ref: "prj_1", Team: "usr_1"}, dir)
	if err != nil || c.Bin != "vercel" || !strings.Contains(strings.Join(c.Env, " "), "VERCEL_PROJECT_ID=prj_1") {
		t.Fatalf("vercel: %+v %v", c, err)
	}
	if strings.Contains(c.Display(), "secret-token") {
		t.Error("the confirm prompt must never show the token")
	}
	if _, err := DeployCommand(Link{Platform: "vercel", Ref: "prj_1"}, dir); err == nil {
		t.Error("vercel link without account id should ask to relink")
	}
	if _, err := DeployCommand(Link{Platform: "netlify"}, dir); err == nil {
		t.Error("unlinked project must not deploy")
	}
	if _, err := DeployCommand(Link{Platform: "cloudflare", Ref: "web"}, dir); err == nil {
		t.Error("cloudflare needs a build folder")
	}
	t.Setenv("CLOUDFLARE_API_TOKEN", "t")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "a")
	os.Mkdir(filepath.Join(dir, "dist"), 0o755)
	c, err = DeployCommand(Link{Platform: "cloudflare", Ref: "web"}, dir)
	if err != nil || c.Args[2] != "dist" || c.Args[4] != "web" {
		t.Fatalf("cloudflare: %+v %v", c, err)
	}
}

// flakyServer drops the connection on POSTs it is told to, optionally after
// recording the project, like a response lost on the way back.
func flakyServer(t *testing.T, dropFirstPOSTs int, createBeforeDrop bool) (*httptest.Server, *[]string) {
	var created []string
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST":
			posts++
			if posts <= dropFirstPOSTs {
				if createBeforeDrop {
					created = append(created, "web")
				}
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
			created = append(created, "web")
			w.Write([]byte(`{"id":"prj_new","name":"web","accountId":"usr_1"}`))
		default: // project list
			out := `{"projects":[`
			for i, n := range created {
				if i > 0 {
					out += ","
				}
				out += `{"id":"prj_existing","name":"` + n + `","accountId":"usr_1"}`
			}
			w.Write([]byte(out + `]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &created
}

func TestCreateSurvivesDroppedConnection(t *testing.T) {
	SetCredsDir(t.TempDir())
	t.Cleanup(func() { SetCredsDir("") })
	t.Setenv("VERCEL_TOKEN", "tok")

	// Dropped before the platform did anything: retried once, created once.
	srv, created := flakyServer(t, 1, false)
	v := &Vercel{base: srv.URL}
	ref, err := createWith(context.Background(), v, v, "web")
	if err != nil || ref.Ref != "prj_new" || len(*created) != 1 {
		t.Fatalf("retry case: %+v err=%v created=%v", ref, err, *created)
	}

	// Created, but the response was lost: adopt it, never create a duplicate.
	srv, created = flakyServer(t, 1, true)
	v = &Vercel{base: srv.URL}
	ref, err = createWith(context.Background(), v, v, "web")
	if err != nil || ref.Ref != "prj_existing" || len(*created) != 1 {
		t.Fatalf("lost-response case: %+v err=%v created=%v", ref, err, *created)
	}

	// Keeps failing: reports the error instead of looping.
	srv, _ = flakyServer(t, 99, false)
	v = &Vercel{base: srv.URL}
	if _, err = createWith(context.Background(), v, v, "web"); err == nil {
		t.Fatal("persistent failure should be reported")
	}

	// HTTP-level refusals are not retried or adopted.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "taken", 409) }))
	defer bad.Close()
	v = &Vercel{base: bad.URL}
	if _, err = createWith(context.Background(), v, v, "web"); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("409 should surface: %v", err)
	}
}

func TestPreflightWarnsAboutUnignoredEnv(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".env", "SECRET=1")
	write(t, dir, ".env.example", "SECRET=")
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skip("git not available")
	}
	if w := PreflightWarnings(dir); len(w) != 1 || !strings.Contains(w[0], ".env is not ignored") {
		t.Fatalf("unignored .env must warn (and .env.example must not): %v", w)
	}
	write(t, dir, ".gitignore", ".env\n")
	if w := PreflightWarnings(dir); len(w) != 0 {
		t.Fatalf("gitignored .env must not warn: %v", w)
	}
	other := t.TempDir()
	write(t, other, ".env.local", "X=1")
	write(t, other, ".vercelignore", ".env.local\n")
	if w := PreflightWarnings(other); len(w) != 0 {
		t.Fatalf("platform-ignored .env must not warn: %v", w)
	}
}
