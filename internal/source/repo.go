package source

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Arnav717/oryn/internal/gitrepo"
)

type Issue struct {
	Number    int
	Title     string
	URL       string
	Author    string
	Created   time.Time
	Updated   time.Time
	Comments  int
	Labels    []string
	Assignees []string
}

type Commit struct {
	SHA     string
	Message string // first line
	Author  string
	URL     string
	At      time.Time
}

// Run is one GitHub Actions workflow run.
type Run struct {
	Number     int
	Workflow   string
	Title      string
	Branch     string
	Event      string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, cancelled, ... (when completed)
	URL        string
	Actor      string
	Created    time.Time
}

func (r Run) Failing() bool { return r.Conclusion == "failure" || r.Conclusion == "timed_out" }

type Release struct {
	Tag, Name, URL string
	At             time.Time
	Prerelease     bool
}

// Person is someone with a stake in the repo: a collaborator with a role
// (visible with push access) and/or a contributor with commit counts.
type Person struct {
	Login         string
	Permission    string // ADMIN, MAINTAIN, WRITE, TRIAGE, READ, or ""
	Contributions int
}

// Alert is an open Dependabot alert.
type Alert struct {
	ID, Package, Ecosystem, Severity, Summary, Manifest, URL string
}

// RepoData is everything oryn shows on a project's GitHub tab.
type RepoData struct {
	Ref           gitrepo.Ref
	Viewer        string // signed-in user's login
	URL           string
	Description   string
	Private       bool
	Fork          bool
	Archived      bool
	Stars, Forks  int
	Language      string
	License       string
	DefaultBranch string
	DefaultCI     string // checks state of the default branch head

	OpenPRs     int // total, PRs holds the most recent
	PRs         []PR
	OpenIssues  int
	Issues      []Issue
	Commits     []Commit
	Runs        []Run
	Releases    []Release
	People      []Person
	Alerts      []Alert
	AlertCount  int
	AlertsKnown bool // false when the token can't see Dependabot alerts
}

const repoQuery = `
query($owner: String!, $name: String!) {
  viewer { login }
  repository(owner: $owner, name: $name) {
    nameWithOwner url description isPrivate isFork isArchived
    stargazerCount forkCount
    primaryLanguage { name }
    licenseInfo { spdxId }
    defaultBranchRef {
      name
      target { ... on Commit {
        oid
        statusCheckRollup { state }
        history(first: 15) { nodes { oid messageHeadline committedDate url author { name user { login } } } }
      } }
    }
    pullRequests(states: OPEN, first: 50, orderBy: {field: UPDATED_AT, direction: DESC}) { totalCount nodes { ...pr } }
    issues(states: OPEN, first: 50, orderBy: {field: UPDATED_AT, direction: DESC}) {
      totalCount
      nodes {
        number title url createdAt updatedAt
        author { login }
        comments { totalCount }
        labels(first: 5) { nodes { name } }
        assignees(first: 3) { nodes { login } }
      }
    }
    releases(first: 8, orderBy: {field: CREATED_AT, direction: DESC}) { nodes { tagName name url publishedAt isPrerelease isDraft } }
    collaborators(first: 50) { edges { permission node { login } } }
    vulnerabilityAlerts(first: 25, states: OPEN) {
      totalCount
      nodes { number vulnerableManifestPath securityVulnerability { severity package { name ecosystem } advisory { summary ghsaId permalink } } }
    }
  }
}` + prFields

type repoResp struct {
	Data *struct {
		Viewer     struct{ Login string }
		Repository *struct {
			NameWithOwner    string
			URL              string
			Description      string
			IsPrivate        bool
			IsFork           bool
			IsArchived       bool
			StargazerCount   int
			ForkCount        int
			PrimaryLanguage  *struct{ Name string }
			LicenseInfo      *struct{ SpdxID string }
			DefaultBranchRef *struct {
				Name   string
				Target struct {
					OID               string
					StatusCheckRollup *struct{ State string }
					History           struct {
						Nodes []struct {
							OID             string
							MessageHeadline string
							CommittedDate   time.Time
							URL             string
							Author          struct {
								Name string
								User *struct{ Login string }
							}
						}
					}
				}
			}
			PullRequests struct {
				TotalCount int
				Nodes      []ghNode
			}
			Issues struct {
				TotalCount int
				Nodes      []struct {
					Number    int
					Title     string
					URL       string
					CreatedAt time.Time
					UpdatedAt time.Time
					Author    *struct{ Login string }
					Comments  struct{ TotalCount int }
					Labels    struct{ Nodes []struct{ Name string } }
					Assignees struct{ Nodes []struct{ Login string } }
				}
			}
			Releases struct {
				Nodes []struct {
					TagName      string
					Name         string
					URL          string
					PublishedAt  time.Time
					IsPrerelease bool
					IsDraft      bool
				}
			}
			Collaborators *struct {
				Edges []struct {
					Permission string
					Node       struct{ Login string }
				}
			}
			VulnerabilityAlerts *struct {
				TotalCount int
				Nodes      []struct {
					Number                 int
					VulnerableManifestPath string
					SecurityVulnerability  struct {
						Severity string
						Package  struct{ Name, Ecosystem string }
						Advisory struct{ Summary, GhsaID, Permalink string }
					}
				}
			}
		}
	}
	Errors []struct{ Message string }
}

// convertRepo turns the GraphQL response into RepoData. A missing repository
// is the only fatal case; fields the token can't see (collaborators, alerts)
// come back null and just leave their sections empty.
func convertRepo(ref gitrepo.Ref, r repoResp) (*RepoData, error) {
	if r.Data == nil || r.Data.Repository == nil {
		msg := "repository not found or not accessible with this token"
		if len(r.Errors) > 0 {
			msg = r.Errors[0].Message
		}
		return nil, fmt.Errorf("github %s: %s", ref, msg)
	}
	g := r.Data.Repository
	d := &RepoData{
		Ref: ref, Viewer: r.Data.Viewer.Login, URL: g.URL, Description: g.Description,
		Private: g.IsPrivate, Fork: g.IsFork, Archived: g.IsArchived,
		Stars: g.StargazerCount, Forks: g.ForkCount,
		OpenPRs: g.PullRequests.TotalCount, OpenIssues: g.Issues.TotalCount,
	}
	if g.PrimaryLanguage != nil {
		d.Language = g.PrimaryLanguage.Name
	}
	if g.LicenseInfo != nil && g.LicenseInfo.SpdxID != "NOASSERTION" {
		d.License = g.LicenseInfo.SpdxID
	}
	if b := g.DefaultBranchRef; b != nil {
		d.DefaultBranch = b.Name
		if b.Target.StatusCheckRollup != nil {
			d.DefaultCI = b.Target.StatusCheckRollup.State
		}
		for _, c := range b.Target.History.Nodes {
			author := c.Author.Name
			if c.Author.User != nil {
				author = c.Author.User.Login
			}
			d.Commits = append(d.Commits, Commit{SHA: c.OID, Message: c.MessageHeadline, Author: author, URL: c.URL, At: c.CommittedDate})
		}
	}
	for _, n := range g.PullRequests.Nodes {
		if n.Number != 0 {
			d.PRs = append(d.PRs, n.pr())
		}
	}
	for _, n := range g.Issues.Nodes {
		is := Issue{Number: n.Number, Title: n.Title, URL: n.URL, Created: n.CreatedAt, Updated: n.UpdatedAt, Comments: n.Comments.TotalCount}
		if n.Author != nil {
			is.Author = n.Author.Login
		}
		for _, l := range n.Labels.Nodes {
			is.Labels = append(is.Labels, l.Name)
		}
		for _, a := range n.Assignees.Nodes {
			is.Assignees = append(is.Assignees, a.Login)
		}
		d.Issues = append(d.Issues, is)
	}
	for _, rel := range g.Releases.Nodes {
		if rel.IsDraft {
			continue
		}
		name := rel.Name
		if name == "" {
			name = rel.TagName
		}
		d.Releases = append(d.Releases, Release{Tag: rel.TagName, Name: name, URL: rel.URL, At: rel.PublishedAt, Prerelease: rel.IsPrerelease})
	}
	if g.Collaborators != nil {
		for _, e := range g.Collaborators.Edges {
			d.People = append(d.People, Person{Login: e.Node.Login, Permission: e.Permission})
		}
	}
	if g.VulnerabilityAlerts != nil {
		d.AlertsKnown = true
		d.AlertCount = g.VulnerabilityAlerts.TotalCount
		for _, a := range g.VulnerabilityAlerts.Nodes {
			v := a.SecurityVulnerability
			d.Alerts = append(d.Alerts, Alert{
				ID: v.Advisory.GhsaID, Package: v.Package.Name, Ecosystem: v.Package.Ecosystem,
				Severity: strings.ToUpper(v.Severity), Summary: v.Advisory.Summary,
				Manifest: a.VulnerableManifestPath, URL: v.Advisory.Permalink,
			})
		}
		sort.SliceStable(d.Alerts, func(i, j int) bool { return SeverityRank(d.Alerts[i].Severity) > SeverityRank(d.Alerts[j].Severity) })
	}
	return d, nil
}

type runsResp struct {
	WorkflowRuns []struct {
		RunNumber    int       `json:"run_number"`
		Name         string    `json:"name"`
		DisplayTitle string    `json:"display_title"`
		HeadBranch   string    `json:"head_branch"`
		Event        string    `json:"event"`
		Status       string    `json:"status"`
		Conclusion   string    `json:"conclusion"`
		HTMLURL      string    `json:"html_url"`
		CreatedAt    time.Time `json:"created_at"`
		Actor        *struct {
			Login string `json:"login"`
		} `json:"actor"`
	} `json:"workflow_runs"`
}

func (r runsResp) runs() []Run {
	out := make([]Run, 0, len(r.WorkflowRuns))
	for _, w := range r.WorkflowRuns {
		run := Run{
			Number: w.RunNumber, Workflow: w.Name, Title: w.DisplayTitle, Branch: w.HeadBranch,
			Event: w.Event, Status: w.Status, Conclusion: w.Conclusion, URL: w.HTMLURL, Created: w.CreatedAt,
		}
		if w.Actor != nil {
			run.Actor = w.Actor.Login
		}
		out = append(out, run)
	}
	return out
}

type contributor struct {
	Login         string `json:"login"`
	Contributions int    `json:"contributions"`
}

var permissionRank = map[string]int{"ADMIN": 5, "MAINTAIN": 4, "WRITE": 3, "TRIAGE": 2, "READ": 1}

// mergePeople combines collaborators (roles) with contributors (commit
// counts), listing people with the most access first.
func mergePeople(collab []Person, contrib []contributor) []Person {
	by := map[string]*Person{}
	var order []string
	get := func(login string) *Person {
		if p, ok := by[login]; ok {
			return p
		}
		by[login] = &Person{Login: login}
		order = append(order, login)
		return by[login]
	}
	for _, c := range collab {
		get(c.Login).Permission = c.Permission
	}
	for _, c := range contrib {
		get(c.Login).Contributions = c.Contributions
	}
	out := make([]Person, 0, len(order))
	for _, l := range order {
		out = append(out, *by[l])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := permissionRank[out[i].Permission], permissionRank[out[j].Permission]; ri != rj {
			return ri > rj
		}
		return out[i].Contributions > out[j].Contributions
	})
	return out
}

func authHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization":        "bearer " + token,
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}
}

// fetchRepo loads one repo. The GraphQL call is required; Actions runs and
// contributors are best-effort extras fetched alongside it.
func fetchRepo(ctx context.Context, token string, ref gitrepo.Ref) (*RepoData, error) {
	var (
		wg      sync.WaitGroup
		resp    repoResp
		mainErr error
		runs    runsResp
		contrib []contributor
	)
	h := authHeaders(token)
	base := fmt.Sprintf("https://api.github.com/repos/%s/%s", ref.Owner, ref.Name)
	wg.Add(3)
	go func() {
		defer wg.Done()
		body := map[string]any{"query": repoQuery, "variables": map[string]string{"owner": ref.Owner, "name": ref.Name}}
		mainErr = doJSONH(ctx, "POST", githubGraphQL, h, body, &resp)
	}()
	go func() {
		defer wg.Done()
		_ = doJSONH(ctx, "GET", base+"/actions/runs?per_page=20", h, nil, &runs)
	}()
	go func() {
		defer wg.Done()
		_ = doJSONH(ctx, "GET", base+"/contributors?per_page=30", h, nil, &contrib)
	}()
	wg.Wait()
	if mainErr != nil {
		return nil, mainErr
	}
	d, err := convertRepo(ref, resp)
	if err != nil {
		return nil, err
	}
	d.Runs = runs.runs()
	d.People = mergePeople(d.People, contrib)
	return d, nil
}

// FetchRepos loads each distinct repo once. Per-repo failures land in the
// error map; the returned error is only ErrNoToken.
func FetchRepos(ctx context.Context, refs []gitrepo.Ref) (map[gitrepo.Ref]*RepoData, map[gitrepo.Ref]error, error) {
	token := GitHubToken()
	if token == "" {
		return nil, nil, ErrNoToken
	}
	seen := map[gitrepo.Ref]bool{}
	var uniq []gitrepo.Ref
	for _, r := range refs {
		if !seen[r] {
			seen[r] = true
			uniq = append(uniq, r)
		}
	}
	data := map[gitrepo.Ref]*RepoData{}
	errs := map[gitrepo.Ref]error{}
	var mu sync.Mutex
	forEach(uniq, 3, func(ref gitrepo.Ref) {
		d, err := fetchRepo(ctx, token, ref)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs[ref] = err
			return
		}
		data[ref] = d
	})
	return data, errs, nil
}
