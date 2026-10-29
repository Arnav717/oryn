// Package gitrepo maps a project folder to its GitHub repository.
package gitrepo

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Ref identifies a GitHub repository.
type Ref struct{ Owner, Name string }

func (r Ref) String() string { return r.Owner + "/" + r.Name }

var (
	segment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// scp-like: git@github.com:owner/name(.git)
	scpLike = regexp.MustCompile(`^[A-Za-z0-9._-]+@([A-Za-z0-9.-]+):(.+)$`)
)

// Parse extracts owner/name from a github.com remote URL in any of the usual
// forms (https, ssh, scp-like, git://). Credentials in the URL are discarded.
// Non-GitHub hosts return ok=false.
func Parse(remote string) (Ref, bool) {
	remote = strings.TrimSpace(remote)
	var host, path string
	if m := scpLike.FindStringSubmatch(remote); m != nil && !strings.Contains(remote, "://") {
		host, path = m[1], m[2]
	} else if i := strings.Index(remote, "://"); i > 0 {
		rest := remote[i+3:]
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return Ref{}, false
		}
		host, path = rest[:slash], rest[slash+1:]
		if at := strings.LastIndex(host, "@"); at >= 0 { // user:token@host
			host = host[at+1:]
		}
		if colon := strings.Index(host, ":"); colon >= 0 { // port
			host = host[:colon]
		}
	} else {
		return Ref{}, false
	}
	switch strings.ToLower(host) {
	case "github.com", "www.github.com", "ssh.github.com":
	default:
		return Ref{}, false
	}
	path = strings.Trim(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || !segment.MatchString(parts[0]) || !segment.MatchString(parts[1]) {
		return Ref{}, false
	}
	return Ref{parts[0], parts[1]}, true
}

// ParseRef parses "owner/name" as written in the config.
func ParseRef(s string) (Ref, bool) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 || !segment.MatchString(parts[0]) || !segment.MatchString(parts[1]) {
		return Ref{}, false
	}
	return Ref{parts[0], parts[1]}, true
}

// Detect finds the GitHub repo for dir from its git remotes, preferring
// "origin". It returns false when dir is not in a git repo, git is missing,
// or no remote points at github.com.
func Detect(dir string) (Ref, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "config", "--get-regexp", `^remote\..*\.url$`).Output()
	if err != nil {
		return Ref{}, false
	}
	return pick(string(out))
}

// pick chooses a repo from `git config --get-regexp` output lines of the form
// "remote.<name>.url <url>".
func pick(config string) (Ref, bool) {
	var first Ref
	found := false
	for _, line := range strings.Split(config, "\n") {
		key, url, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		ref, ok := Parse(url)
		if !ok {
			continue
		}
		if key == "remote.origin.url" {
			return ref, true
		}
		if !found {
			first, found = ref, true
		}
	}
	return first, found
}
