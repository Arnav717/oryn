package gitrepo

import "testing"

func TestParse(t *testing.T) {
	good := map[string]string{
		"https://github.com/Arnav717/oryn.git":       "Arnav717/oryn",
		"https://github.com/Arnav717/oryn":           "Arnav717/oryn",
		"https://github.com/Arnav717/oryn/":          "Arnav717/oryn",
		"http://github.com/o/r.git":                  "o/r",
		"git@github.com:o/r.git":                     "o/r",
		"git@github.com:o/my.repo":                   "o/my.repo",
		"ssh://git@github.com/o/r.git":               "o/r",
		"ssh://git@github.com:22/o/r.git":            "o/r",
		"git://github.com/o/r.git":                   "o/r",
		"https://user:ghp_secret@github.com/o/r.git": "o/r",
		"  https://GitHub.com/o/r.git \n":            "o/r",
		"git@ssh.github.com:o/r.git":                 "o/r",
	}
	for in, want := range good {
		ref, ok := Parse(in)
		if !ok || ref.String() != want {
			t.Errorf("Parse(%q) = %v, %v; want %s", in, ref, ok, want)
		}
	}
	for _, in := range []string{
		"", "not a url", "https://gitlab.com/o/r.git", "git@gitlab.com:o/r.git",
		"https://github.com/o", "https://github.com/o/r/extra", "https://github.com/o/r with space",
		"https://evilgithub.com/o/r", "https://github.com.evil.com/o/r", "/local/path/repo",
	} {
		if ref, ok := Parse(in); ok {
			t.Errorf("Parse(%q) = %v, want no match", in, ref)
		}
	}
}

func TestParseRef(t *testing.T) {
	if r, ok := ParseRef(" o/r "); !ok || r.String() != "o/r" {
		t.Errorf("ParseRef = %v %v", r, ok)
	}
	for _, in := range []string{"", "o", "o/r/x", "o/ r", "/r"} {
		if _, ok := ParseRef(in); ok {
			t.Errorf("ParseRef(%q) should fail", in)
		}
	}
}

func TestPickPrefersOrigin(t *testing.T) {
	cfg := "remote.upstream.url https://github.com/up/stream.git\nremote.origin.url git@github.com:me/fork.git\n"
	if r, ok := pick(cfg); !ok || r.String() != "me/fork" {
		t.Errorf("pick = %v %v", r, ok)
	}
	// No origin on GitHub: fall back to the first GitHub remote.
	cfg = "remote.origin.url https://gitlab.com/a/b.git\nremote.mirror.url https://github.com/m/r.git\n"
	if r, ok := pick(cfg); !ok || r.String() != "m/r" {
		t.Errorf("fallback pick = %v %v", r, ok)
	}
	if _, ok := pick("remote.origin.url https://gitlab.com/a/b.git\n"); ok {
		t.Error("gitlab-only should not match")
	}
}

func TestDetectThisRepo(t *testing.T) {
	// Not asserting the remote (CI clones differ); just that a non-repo is safe.
	if ref, ok := Detect(t.TempDir()); ok {
		t.Errorf("empty temp dir detected as %v", ref)
	}
}
