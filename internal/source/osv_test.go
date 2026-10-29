package source

import "testing"

func TestFixedVersion(t *testing.T) {
	var r osvRecord
	r.Affected = make([]struct {
		Package osvPkg `json:"package"`
		Ranges  []struct {
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
	}, 1)
	r.Affected[0].Package = osvPkg{Name: "lodash", Ecosystem: "npm"}
	r.Affected[0].Ranges = []struct {
		Events []map[string]string `json:"events"`
	}{
		{Events: []map[string]string{{"introduced": "0"}, {"fixed": "4.17.12"}}},
		{Events: []map[string]string{{"introduced": "4.17.13"}, {"fixed": "4.17.21"}}},
		{Events: []map[string]string{{"introduced": "5.0.0"}, {"fixed": "5.0.3"}}},
	}

	cases := map[string]string{"4.17.15": "4.17.21", "4.17.0": "4.17.12", "5.0.1": "5.0.3"}
	for cur, want := range cases {
		if got := fixedVersion(r, osvQuery{osvPkg{"lodash", "npm"}, cur}); got != want {
			t.Errorf("fixedVersion(%s) = %q, want %q", cur, got, want)
		}
	}
}

func TestDedupeKeepsMostSevere(t *testing.T) {
	vs := dedupe([]Vuln{
		{ID: "GO-2024-1", Aliases: []string{"CVE-2024-1"}, Severity: "UNKNOWN"},
		{ID: "GHSA-x", Aliases: []string{"CVE-2024-1"}, Severity: "HIGH"},
		{ID: "GHSA-y", Severity: "LOW"},
	})
	if len(vs) != 2 || vs[0].ID != "GHSA-x" {
		t.Fatalf("got %+v", vs)
	}
}

func TestOutdated(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"v1.2.3", "v1.2.3", false},
		{"1.2.3", "v1.2.3", false},
		{"1.2.3", "1.3.0", true},
		{"", "1.0.0", false},
		{"1.0.0", "", false},
	}
	for _, c := range cases {
		if got := Outdated(c.cur, c.latest); got != c.want {
			t.Errorf("Outdated(%q, %q) = %v", c.cur, c.latest, got)
		}
	}
}

func TestCrateIndexPath(t *testing.T) {
	cases := map[string]string{
		"a":     "1/a",
		"ab":    "2/ab",
		"abc":   "3/a/abc",
		"serde": "se/rd/serde",
		"Tokio": "to/ki/tokio",
		"abcd":  "ab/cd/abcd",
	}
	for in, want := range cases {
		if got := crateIndexPath(in); got != want {
			t.Errorf("crateIndexPath(%q) = %q, want %q", in, got, want)
		}
	}
}
