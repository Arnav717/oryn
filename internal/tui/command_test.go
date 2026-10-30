package tui

import "testing"

func TestShortName(t *testing.T) {
	cases := map[string]string{
		"github.com/go-chi/chi/v5": "chi",
		"@types/node":              "node",
		"lodash":                   "lodash",
		"golang.org/x/crypto":      "crypto",
		"v8":                       "v8",
	}
	for in, want := range cases {
		if got := shortName(in); got != want {
			t.Errorf("shortName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatches(t *testing.T) {
	m := Model{pkgs: map[string]*pkgInfo{}}
	for _, n := range []string{"lodash", "lodash.merge", "github.com/go-chi/chi/v5", "react", "react-dom", "chalk"} {
		m.pkgs["npm/"+n] = &pkgInfo{Eco: "npm", Name: n, Direct: true}
		m.pkgOrder = append(m.pkgOrder, "npm/"+n)
	}
	cases := map[string]string{
		"lodash": "npm/lodash",                   // exact beats prefix
		"chi":    "npm/github.com/go-chi/chi/v5", // short name exact
		"rea":    "npm/react",                    // prefix, package order
		"rdom":   "npm/react-dom",                // subsequence
	}
	for q, want := range cases {
		got := m.matches(q, 1)
		if len(got) == 0 || got[0] != want {
			t.Errorf("matches(%q) = %v, want %s", q, got, want)
		}
	}
	if got := m.matches("zzz", 5); len(got) != 0 {
		t.Errorf("matches(zzz) = %v, want none", got)
	}
}
