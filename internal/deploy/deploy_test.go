package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	write(t, root, "wrangler.jsonc", `{ // worker
  "name": "edge-api" }`)
	write(t, root, "fly.toml", `app = "my-fly"`)

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
	if got["cloudflare"].Ref != "edge-api" || got["fly"].Ref != "my-fly" {
		t.Errorf("cloudflare/fly: %+v %+v", got["cloudflare"], got["fly"])
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
