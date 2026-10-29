package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const githubGraphQL = "https://api.github.com/graphql"

// ErrNoToken means no GitHub credentials were found; callers treat it as
// "not connected" rather than a failure.
var ErrNoToken = errors.New("no GitHub token (set GH_TOKEN or run `gh auth login`)")

// PR is an open pull request relevant to the signed-in user.
type PR struct {
	Repo      string // owner/name
	Number    int
	Title     string
	URL       string
	Author    string
	Draft     bool
	Updated   time.Time
	Review    string // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, or ""
	CI        string // SUCCESS, FAILURE, ERROR, PENDING, EXPECTED, or "" when no checks
	CommitOID string // head commit the CI state belongs to
}

func (p PR) Failing() bool { return p.CI == "FAILURE" || p.CI == "ERROR" }

// Ref is the short "owner/name#123" form.
func (p PR) Ref() string { return fmt.Sprintf("%s#%d", p.Repo, p.Number) }

type GitHubData struct {
	Login  string
	Review []PR // review requested from you
	Mine   []PR // authored by you
}

// GitHubToken finds a token without storing anything: GH_TOKEN, GITHUB_TOKEN,
// then the gh CLI's own login.
func GitHubToken() string {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	if p, err := exec.LookPath("gh"); err == nil {
		if out, err := exec.Command(p, "auth", "token").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

// prFields is the PR selection shared by the account-wide and per-repo queries.
const prFields = `
fragment pr on PullRequest {
  number title url isDraft updatedAt reviewDecision
  repository { nameWithOwner }
  author { login }
  commits(last: 1) { nodes { commit { oid statusCheckRollup { state } } } }
}`

const githubQuery = `
query($mine: String!, $review: String!) {
  viewer { login }
  mine: search(query: $mine, type: ISSUE, first: 50) { nodes { ...pr } }
  review: search(query: $review, type: ISSUE, first: 50) { nodes { ...pr } }
}` + prFields

type ghNode struct {
	Number         int
	Title          string
	URL            string
	IsDraft        bool
	UpdatedAt      time.Time
	ReviewDecision string
	Repository     struct{ NameWithOwner string }
	Author         *struct{ Login string }
	Commits        struct {
		Nodes []struct {
			Commit struct {
				OID               string
				StatusCheckRollup *struct{ State string }
			}
		}
	}
}

type ghResponse struct {
	Data *struct {
		Viewer struct{ Login string }
		Mine   struct{ Nodes []ghNode }
		Review struct{ Nodes []ghNode }
	}
	Errors []struct{ Message string }
}

func (n ghNode) pr() PR {
	p := PR{
		Repo:    n.Repository.NameWithOwner,
		Number:  n.Number,
		Title:   n.Title,
		URL:     n.URL,
		Draft:   n.IsDraft,
		Updated: n.UpdatedAt,
		Review:  n.ReviewDecision,
	}
	if n.Author != nil {
		p.Author = n.Author.Login
	}
	if len(n.Commits.Nodes) > 0 {
		c := n.Commits.Nodes[0].Commit
		p.CommitOID = c.OID
		if c.StatusCheckRollup != nil {
			p.CI = c.StatusCheckRollup.State
		}
	}
	return p
}

// convertGitHub turns a GraphQL response into GitHubData. Errors are only
// fatal when no data came back (SAML-protected orgs, for one, produce partial
// errors alongside usable data). Non-PR search hits decode as empty nodes.
func convertGitHub(r ghResponse) (*GitHubData, error) {
	if r.Data == nil {
		if len(r.Errors) > 0 {
			return nil, fmt.Errorf("github: %s", r.Errors[0].Message)
		}
		return nil, errors.New("github: empty response")
	}
	conv := func(nodes []ghNode) []PR {
		var out []PR
		for _, n := range nodes {
			if n.Number != 0 {
				out = append(out, n.pr())
			}
		}
		return out
	}
	d := &GitHubData{Login: r.Data.Viewer.Login, Review: conv(r.Data.Review.Nodes), Mine: conv(r.Data.Mine.Nodes)}
	for i := range d.Mine {
		if d.Mine[i].Author == "" {
			d.Mine[i].Author = d.Login
		}
	}
	return d, nil
}

// FetchGitHub loads your review queue and your open PRs with CI state in a
// single GraphQL request. The token only ever goes to api.github.com.
func FetchGitHub(ctx context.Context) (*GitHubData, error) {
	token := GitHubToken()
	if token == "" {
		return nil, ErrNoToken
	}
	body := map[string]any{
		"query": githubQuery,
		"variables": map[string]string{
			"mine":   "is:pr is:open author:@me archived:false sort:updated-desc",
			"review": "is:pr is:open review-requested:@me archived:false sort:updated-desc",
		},
	}
	var resp ghResponse
	err := doJSONH(ctx, "POST", githubGraphQL, map[string]string{"Authorization": "bearer " + token}, body, &resp)
	if err != nil {
		return nil, err
	}
	return convertGitHub(resp)
}
