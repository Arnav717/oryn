package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Arnav717/oryn/internal/deploy"
	"github.com/Arnav717/oryn/internal/gitrepo"
	"github.com/Arnav717/oryn/internal/source"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	SetCacheDir(dir)
	defer func() { cachePath = "" }()
	if LoadCache() != nil {
		t.Fatal("empty cache should be nil")
	}
	in := &Snapshot{
		At:       time.Now().Round(0),
		RepoRefs: map[string]gitrepo.Ref{"/p": {Owner: "o", Name: "r"}},
		Repos:    map[string]*source.RepoData{"/p": {OpenPRs: 3, DefaultCI: "SUCCESS"}},
		RepoErrs: map[string]error{"/x": errors.New("boom")},
		Deploys:  []DeployTarget{{Link: deploy.Link{Platform: "vercel"}, LiveURL: "https://x.dev", Err: errors.New("nope")}},
		Status:   []source.Status{{Service: "GitHub", Err: errors.New("down")}},
	}
	in.GitHubErr = errors.New("no")
	if err := SaveCache(in); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "snapshot.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	out := LoadCache()
	if out == nil || out.Repos["/p"].OpenPRs != 3 || out.Deploys[0].LiveURL != "https://x.dev" || out.RepoRefs["/p"].Name != "r" {
		t.Fatalf("got %+v", out)
	}

	os.WriteFile(filepath.Join(dir, "snapshot.json"), []byte("{broken"), 0o600)
	if LoadCache() != nil {
		t.Error("a bad cache should read as none")
	}
}
