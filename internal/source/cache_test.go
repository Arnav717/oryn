package source

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/detect"
)

func testCaches(t *testing.T) {
	t.Helper()
	old := cacheDir
	cacheDir = t.TempDir()
	reset := func() {
		osvRecords.items, osvMatches.items, latestVersions.items = nil, nil, nil
	}
	reset()
	t.Cleanup(func() { cacheDir = old; reset() })
}

func TestOSVCaching(t *testing.T) {
	testCaches(t)
	var batches, details atomic.Int32
	vulnerable := []string{"V-1"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/querybatch" {
			batches.Add(1)
			s := ""
			for i, id := range vulnerable {
				if i > 0 {
					s += ","
				}
				s += fmt.Sprintf(`{"id":%q}`, id)
			}
			fmt.Fprintf(w, `{"results":[{"vulns":[%s]}]}`, s)
			return
		}
		details.Add(1)
		fmt.Fprintf(w, `{"id":%q,"summary":"bad"}`, r.URL.Path[len("/v1/vulns/"):])
	}))
	defer srv.Close()
	old := osvAPI
	osvAPI = srv.URL + "/v1"
	defer func() { osvAPI = old }()

	deps := []detect.Dep{{Ecosystem: "npm", Name: "x", Version: "1.0.0"}}
	for range 3 {
		got, err := QueryOSV(context.Background(), deps)
		if err != nil || len(got[DepKey(deps[0])]) != 1 {
			t.Fatalf("got %+v, %v", got, err)
		}
	}
	if batches.Load() != 1 || details.Load() != 1 {
		t.Errorf("batches %d, details %d", batches.Load(), details.Load())
	}

	// A lockfile change is noticed at once.
	changed := append(deps, detect.Dep{Ecosystem: "npm", Name: "y", Version: "2.0.0"})
	QueryOSV(context.Background(), changed)
	if batches.Load() != 2 {
		t.Errorf("changed deps should ask again: %d", batches.Load())
	}

	// Once the short window passes, a new vulnerability shows up.
	osvMatches.items = nil
	os.Remove(osvMatches.path())
	vulnerable = []string{"V-1", "V-2"}
	got, _ := QueryOSV(context.Background(), deps)
	if len(got[DepKey(deps[0])]) != 2 || details.Load() != 2 {
		t.Errorf("new vuln: %+v, %d detail calls", got, details.Load())
	}

	// Details survive a restart.
	osvRecords.items, osvMatches.items = nil, nil
	os.Remove(osvMatches.path())
	QueryOSV(context.Background(), deps)
	if details.Load() != 2 {
		t.Errorf("not read back from disk: %d", details.Load())
	}
}

func TestOSVOldMatchesBeatAFailure(t *testing.T) {
	testCaches(t)
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", 503)
			return
		}
		if r.URL.Path == "/v1/querybatch" {
			fmt.Fprint(w, `{"results":[{"vulns":[{"id":"V-1"}]}]}`)
			return
		}
		fmt.Fprint(w, `{"id":"V-1","summary":"bad"}`)
	}))
	defer srv.Close()
	old := osvAPI
	osvAPI = srv.URL + "/v1"
	defer func() { osvAPI = old }()
	deps := []detect.Dep{{Ecosystem: "npm", Name: "x", Version: "1.0.0"}}
	if _, err := QueryOSV(context.Background(), deps); err != nil {
		t.Fatal(err)
	}
	for k, e := range osvMatches.items {
		e.At = e.At.Add(-time.Hour)
		osvMatches.items[k] = e
	}
	fail = true
	got, err := QueryOSV(context.Background(), deps)
	if err != nil || len(got[DepKey(deps[0])]) != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestOSVStaleDetailsBeatAFailure(t *testing.T) {
	testCaches(t)
	osvRecords.put("V-1", osvRecord{ID: "V-1", Summary: "old"}, time.Now().Add(-30*24*time.Hour))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/querybatch" {
			fmt.Fprint(w, `{"results":[{"vulns":[{"id":"V-1"}]}]}`)
			return
		}
		http.Error(w, "down", 503)
	}))
	defer srv.Close()
	old := osvAPI
	osvAPI = srv.URL + "/v1"
	defer func() { osvAPI = old }()
	deps := []detect.Dep{{Ecosystem: "npm", Name: "x", Version: "1.0.0"}}
	got, err := QueryOSV(context.Background(), deps)
	if err != nil || len(got[DepKey(deps[0])]) != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestLatestIsCached(t *testing.T) {
	testCaches(t)
	var calls atomic.Int32
	fail := false
	oldFn := latestFn
	latestFn = func(_ context.Context, _, _ string) (string, error) {
		calls.Add(1)
		if fail {
			return "", errors.New("down")
		}
		return "2.0.0", nil
	}
	defer func() { latestFn = oldFn }()
	deps := []detect.Dep{{Ecosystem: "npm", Name: "x", Direct: true}}
	for range 3 {
		if got := LatestAll(context.Background(), deps); got["npm/x"] != "2.0.0" {
			t.Fatalf("got %v", got)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("calls %d", calls.Load())
	}

	// Past its TTL it asks again, and a failure keeps the old answer.
	latestVersions.put("npm/x", "1.5.0", time.Now().Add(-48*time.Hour))
	fail = true
	if got := LatestAll(context.Background(), deps); got["npm/x"] != "1.5.0" || calls.Load() != 2 {
		t.Errorf("got %v after %d calls", got, calls.Load())
	}
}

func TestCorruptCacheFileIsIgnored(t *testing.T) {
	testCaches(t)
	if err := os.WriteFile(osvRecords.path(), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := osvRecords.get("a", time.Now()); found {
		t.Error("found something in a broken file")
	}
	osvRecords.put("a", osvRecord{ID: "a"}, time.Now())
	if err := osvRecords.save(); err != nil {
		t.Fatal(err)
	}
}
