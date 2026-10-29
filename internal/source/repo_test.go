package source

import (
	"encoding/json"
	"testing"

	"github.com/Arnav717/oryn/internal/gitrepo"
)

var testRef = gitrepo.Ref{Owner: "o", Name: "r"}

const repoFixture = `{
 "data": {
  "viewer": {"login": "me"},
  "repository": {
   "nameWithOwner": "o/r", "url": "https://github.com/o/r", "description": "a repo",
   "isPrivate": true, "isFork": false, "isArchived": false,
   "stargazerCount": 12, "forkCount": 3,
   "primaryLanguage": {"name": "Go"}, "licenseInfo": {"spdxId": "MIT"},
   "defaultBranchRef": {"name": "main", "target": {
     "oid": "aaa", "statusCheckRollup": {"state": "FAILURE"},
     "history": {"nodes": [
       {"oid": "abc1234567", "messageHeadline": "fix: thing", "committedDate": "2026-10-28T10:00:00Z",
        "url": "https://github.com/o/r/commit/abc", "author": {"name": "Bob B", "user": {"login": "bob"}}},
       {"oid": "def7654321", "messageHeadline": "chore", "committedDate": "2026-10-27T10:00:00Z",
        "url": "https://github.com/o/r/commit/def", "author": {"name": "No Account", "user": null}}
     ]}}},
   "pullRequests": {"totalCount": 7, "nodes": [
     {"number": 5, "title": "Add x", "url": "https://github.com/o/r/pull/5", "isDraft": false,
      "updatedAt": "2026-10-28T10:00:00Z", "reviewDecision": "APPROVED",
      "repository": {"nameWithOwner": "o/r"}, "author": {"login": "bob"},
      "commits": {"nodes": [{"commit": {"oid": "bbb", "statusCheckRollup": {"state": "SUCCESS"}}}]}}
   ]},
   "issues": {"totalCount": 31, "nodes": [
     {"number": 9, "title": "Crash on start", "url": "https://github.com/o/r/issues/9",
      "createdAt": "2026-10-01T10:00:00Z", "updatedAt": "2026-10-20T10:00:00Z",
      "author": null, "comments": {"totalCount": 4},
      "labels": {"nodes": [{"name": "bug"}, {"name": "p1"}]}, "assignees": {"nodes": [{"login": "me"}]}}
   ]},
   "releases": {"nodes": [
     {"tagName": "v1.0.0", "name": "", "url": "https://github.com/o/r/releases/tag/v1.0.0",
      "publishedAt": "2026-09-01T10:00:00Z", "isPrerelease": false, "isDraft": false},
     {"tagName": "v2.0.0-rc", "name": "RC", "url": "u", "publishedAt": "2026-10-01T10:00:00Z", "isPrerelease": true, "isDraft": true}
   ]},
   "collaborators": {"edges": [{"permission": "ADMIN", "node": {"login": "me"}}, {"permission": "WRITE", "node": {"login": "bob"}}]},
   "vulnerabilityAlerts": {"totalCount": 2, "nodes": [
     {"number": 1, "vulnerableManifestPath": "go.mod", "securityVulnerability": {"severity": "MODERATE",
       "package": {"name": "x/y", "ecosystem": "GO"}, "advisory": {"summary": "meh", "ghsaId": "GHSA-1", "permalink": "https://github.com/advisories/GHSA-1"}}},
     {"number": 2, "vulnerableManifestPath": "go.mod", "securityVulnerability": {"severity": "CRITICAL",
       "package": {"name": "a/b", "ecosystem": "GO"}, "advisory": {"summary": "bad", "ghsaId": "GHSA-2", "permalink": "https://github.com/advisories/GHSA-2"}}}
   ]}
  }
 }
}`

func TestConvertRepo(t *testing.T) {
	var r repoResp
	if err := json.Unmarshal([]byte(repoFixture), &r); err != nil {
		t.Fatal(err)
	}
	d, err := convertRepo(testRef, r)
	if err != nil {
		t.Fatal(err)
	}
	if d.Viewer != "me" || !d.Private || d.Stars != 12 || d.Language != "Go" || d.License != "MIT" || d.DefaultBranch != "main" || d.DefaultCI != "FAILURE" {
		t.Errorf("overview: %+v", d)
	}
	if d.OpenPRs != 7 || len(d.PRs) != 1 || d.PRs[0].Ref() != "o/r#5" || d.PRs[0].CI != "SUCCESS" || d.PRs[0].Review != "APPROVED" {
		t.Errorf("prs: %d %+v", d.OpenPRs, d.PRs)
	}
	if d.OpenIssues != 31 || len(d.Issues) != 1 || d.Issues[0].Comments != 4 || d.Issues[0].Author != "" ||
		len(d.Issues[0].Labels) != 2 || d.Issues[0].Assignees[0] != "me" {
		t.Errorf("issues: %d %+v", d.OpenIssues, d.Issues)
	}
	if len(d.Commits) != 2 || d.Commits[0].Author != "bob" || d.Commits[1].Author != "No Account" {
		t.Errorf("commits: %+v", d.Commits)
	}
	if len(d.Releases) != 1 || d.Releases[0].Name != "v1.0.0" { // draft skipped, empty name falls back to tag
		t.Errorf("releases: %+v", d.Releases)
	}
	if !d.AlertsKnown || d.AlertCount != 2 || d.Alerts[0].Severity != "CRITICAL" || d.Alerts[1].Severity != "MODERATE" {
		t.Errorf("alerts: %+v", d.Alerts)
	}
	if len(d.People) != 2 || d.People[0].Permission != "ADMIN" {
		t.Errorf("people: %+v", d.People)
	}
}

func TestConvertRepoWithoutPermissions(t *testing.T) {
	// collaborators and vulnerabilityAlerts are null without admin access.
	var r repoResp
	json.Unmarshal([]byte(`{"data":{"viewer":{"login":"me"},"repository":{"nameWithOwner":"o/r","url":"u",
	  "pullRequests":{"totalCount":0,"nodes":[]},"issues":{"totalCount":0,"nodes":[]},"releases":{"nodes":[]},
	  "collaborators":null,"vulnerabilityAlerts":null}},
	  "errors":[{"message":"Resource not accessible"}]}`), &r)
	d, err := convertRepo(testRef, r)
	if err != nil {
		t.Fatalf("partial errors must not be fatal: %v", err)
	}
	if d.AlertsKnown || len(d.People) != 0 || d.DefaultBranch != "" {
		t.Errorf("got %+v", d)
	}
}

func TestConvertRepoNotFound(t *testing.T) {
	var r repoResp
	json.Unmarshal([]byte(`{"data":{"viewer":{"login":"me"},"repository":null},"errors":[{"message":"Could not resolve to a Repository with the name 'o/r'."}]}`), &r)
	if _, err := convertRepo(testRef, r); err == nil {
		t.Error("missing repository should be an error")
	}
}

func TestMergePeople(t *testing.T) {
	got := mergePeople(
		[]Person{{Login: "bob", Permission: "WRITE"}, {Login: "me", Permission: "ADMIN"}},
		[]contributor{{"carol", 50}, {"bob", 10}, {"me", 3}},
	)
	order := []string{"me", "bob", "carol"} // role first, then commits
	for i, l := range order {
		if got[i].Login != l {
			t.Fatalf("order = %+v", got)
		}
	}
	if got[1].Contributions != 10 || got[1].Permission != "WRITE" || got[2].Permission != "" {
		t.Errorf("merge: %+v", got)
	}
}

func TestRuns(t *testing.T) {
	var r runsResp
	json.Unmarshal([]byte(`{"workflow_runs":[
	  {"run_number":12,"name":"CI","display_title":"fix","head_branch":"main","event":"push","status":"completed","conclusion":"failure","html_url":"u","created_at":"2026-10-28T10:00:00Z","actor":{"login":"bob"}},
	  {"run_number":13,"name":"CI","display_title":"wip","head_branch":"dev","event":"pull_request","status":"in_progress","conclusion":null,"html_url":"u2","created_at":"2026-10-29T10:00:00Z","actor":null}]}`), &r)
	runs := r.runs()
	if len(runs) != 2 || !runs[0].Failing() || runs[0].Actor != "bob" || runs[1].Failing() || runs[1].Status != "in_progress" {
		t.Errorf("runs: %+v", runs)
	}
}
