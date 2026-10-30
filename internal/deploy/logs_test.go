package deploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// record serves body and keeps each request's method and URI.
func record(t *testing.T, calls *[]string, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.RequestURI())
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestVercelLogsReadBothEventShapes(t *testing.T) {
	t.Setenv("VERCEL_TOKEN", "tok")
	var calls []string
	srv := record(t, &calls, `[{"type":"stdout","text":"one"},{"type":"stderr","payload":{"text":"two"}},{"type":"delimiter"}]`)
	got, err := (&Vercel{base: srv.URL}).Logs(context.Background(), Link{Team: "team_9"}, "dpl_1")
	if err != nil || !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("got %v err %v", got, err)
	}
	want := "GET /v3/deployments/dpl_1/events?builds=1&direction=forward&limit=-1&teamId=team_9"
	if calls[0] != want {
		t.Errorf("called %s", calls[0])
	}
}

func TestCloudflareLogs(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acc")
	var calls []string
	srv := record(t, &calls, `{"result":{"data":[{"line":"building","ts":"2026-10-30T10:00:00Z"}],"total":1}}`)
	got, err := (&Cloudflare{base: srv.URL}).Logs(context.Background(), Link{Ref: "site"}, "d1")
	if err != nil || !reflect.DeepEqual(got, []string{"building"}) {
		t.Fatalf("got %v err %v", got, err)
	}
	if calls[0] != "GET /client/v4/accounts/acc/pages/projects/site/deployments/d1/history/logs" {
		t.Errorf("called %s", calls[0])
	}
}

func TestRollbackPaths(t *testing.T) {
	t.Setenv("VERCEL_TOKEN", "tok")
	t.Setenv("NETLIFY_AUTH_TOKEN", "tok")
	t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acc")
	var calls []string
	srv := record(t, &calls, `{}`)
	ctx := context.Background()
	l := Link{Ref: "prj", Team: "team_9"}
	for _, r := range []Rollbacker{&Vercel{base: srv.URL}, &Netlify{base: srv.URL}, &Cloudflare{base: srv.URL}} {
		if err := r.Rollback(ctx, l, "d1"); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"POST /v1/projects/prj/rollback/d1?teamId=team_9",
		"POST /api/v1/sites/prj/deploys/d1/restore",
		"POST /client/v4/accounts/acc/pages/projects/prj/deployments/d1/rollback",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("got %v", calls)
	}
}

func TestPlatformSupport(t *testing.T) {
	if LogReaderFor("netlify") != nil {
		t.Error("Netlify's API has no build log to read")
	}
	if LogReaderFor("vercel") == nil || LogReaderFor("cloudflare") == nil {
		t.Error("vercel and cloudflare should read logs")
	}
	for _, p := range Platforms {
		if RollbackerFor(p) == nil {
			t.Errorf("%s should roll back", p)
		}
	}
	if LogReaderFor("heroku") != nil || RollbackerFor("heroku") != nil {
		t.Error("an unknown platform supports nothing")
	}
}
