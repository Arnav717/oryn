package source

import (
	"encoding/json"
	"testing"
)

const ghFixture = `{
  "data": {
    "viewer": {"login": "me"},
    "mine": {"nodes": [
      {"number": 7, "title": "Fix it", "url": "https://github.com/o/r/pull/7", "isDraft": false,
       "updatedAt": "2026-10-28T10:00:00Z", "reviewDecision": "CHANGES_REQUESTED",
       "repository": {"nameWithOwner": "o/r"}, "author": null,
       "commits": {"nodes": [{"commit": {"oid": "abc1234", "statusCheckRollup": {"state": "FAILURE"}}}]}}
    ]},
    "review": {"nodes": [
      {},
      {"number": 9, "title": "Add thing", "url": "https://github.com/o/x/pull/9", "isDraft": true,
       "updatedAt": "2026-10-29T08:00:00Z", "reviewDecision": null,
       "repository": {"nameWithOwner": "o/x"}, "author": {"login": "bob"},
       "commits": {"nodes": [{"commit": {"oid": "def5678", "statusCheckRollup": null}}]}}
    ]}
  }
}`

func TestConvertGitHub(t *testing.T) {
	var r ghResponse
	if err := json.Unmarshal([]byte(ghFixture), &r); err != nil {
		t.Fatal(err)
	}
	d, err := convertGitHub(r)
	if err != nil {
		t.Fatal(err)
	}
	if d.Login != "me" || len(d.Mine) != 1 || len(d.Review) != 1 {
		t.Fatalf("got %+v", d)
	}
	m := d.Mine[0]
	if m.Ref() != "o/r#7" || !m.Failing() || m.Author != "me" || m.Review != "CHANGES_REQUESTED" || m.CommitOID != "abc1234" {
		t.Errorf("mine: %+v", m)
	}
	rv := d.Review[0]
	if rv.Ref() != "o/x#9" || rv.Failing() || !rv.Draft || rv.Author != "bob" || rv.CI != "" {
		t.Errorf("review: %+v", rv)
	}
}

func TestConvertGitHubErrors(t *testing.T) {
	var r ghResponse
	json.Unmarshal([]byte(`{"errors":[{"message":"Bad credentials"}]}`), &r)
	if _, err := convertGitHub(r); err == nil || err.Error() != "github: Bad credentials" {
		t.Errorf("err = %v", err)
	}
	// Partial errors with usable data are not fatal.
	json.Unmarshal([]byte(`{"data":{"viewer":{"login":"me"},"mine":{"nodes":[]},"review":{"nodes":[]}},"errors":[{"message":"SAML"}]}`), &r)
	if d, err := convertGitHub(r); err != nil || d.Login != "me" {
		t.Errorf("partial: %v %v", d, err)
	}
}
