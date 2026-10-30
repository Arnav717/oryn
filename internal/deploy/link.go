package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/Arnav717/oryn/internal/source"
)

// ProjectRef is a project/site the connected account can see.
type ProjectRef struct {
	Name string
	Ref  string // identifier used by List
	Team string
}

// Connector can check a token and list the account's projects, which is what
// the UI needs to link an account and bind a folder to a project.
type Connector interface {
	Provider
	// Verify checks token against the platform. who is a display name; extra
	// is a platform-specific value to store alongside (Cloudflare account id).
	Verify(ctx context.Context, token string) (who, extra string, err error)
	Projects(ctx context.Context) ([]ProjectRef, error)
}

// TokenPage is where a user creates an API token for the platform.
var TokenPage = map[string]string{
	"vercel":     "https://vercel.com/account/tokens",
	"netlify":    "https://app.netlify.com/user/applications#personal-access-tokens",
	"cloudflare": "https://dash.cloudflare.com/profile/api-tokens",
}

// Platforms lists the supported platforms in display order.
var Platforms = []string{"vercel", "netlify", "cloudflare"}

func ConnectorFor(platform string) Connector {
	switch platform {
	case "vercel":
		return NewVercel()
	case "netlify":
		return NewNetlify()
	case "cloudflare":
		return NewCloudflare()
	}
	return nil
}

// Connect verifies token and, only if it works, stores it.
func Connect(ctx context.Context, platform, token string) (string, error) {
	return connect(ctx, ConnectorFor(platform), platform, token)
}

func connect(ctx context.Context, c Connector, platform, token string) (string, error) {
	token = strings.TrimSpace(token)
	if c == nil {
		return "", fmt.Errorf("unknown platform %q", platform)
	}
	if token == "" {
		return "", errors.New("empty token")
	}
	who, extra, err := c.Verify(ctx, token)
	if err != nil {
		return "", fmt.Errorf("%s rejected the token: %w", platform, errClean(err))
	}
	kv := map[string]string{platform + "_token": token}
	if platform == "cloudflare" && extra != "" {
		kv["cloudflare_account"] = extra
	}
	if err := storeValues(kv); err != nil {
		return "", err
	}
	return who, nil
}

// errClean keeps HTTP errors short for a one-line status message.
func errClean(err error) error {
	msg := err.Error()
	if len(msg) > 120 {
		msg = msg[:120] + "…"
	}
	return errors.New(msg)
}

// Source reports where the platform's credentials come from: "env", "oryn"
// (stored by the UI), "cli" (the platform CLI's login) or "" when missing.
func Source(platform string) string {
	switch platform {
	case "vercel":
		if os.Getenv("VERCEL_TOKEN") != "" {
			return "env"
		}
		if stored("vercel_token") != "" {
			return "oryn"
		}
		if vercelToken() != "" {
			return "cli"
		}
	case "netlify":
		if os.Getenv("NETLIFY_AUTH_TOKEN") != "" {
			return "env"
		}
		if stored("netlify_token") != "" {
			return "oryn"
		}
		if netlifyToken() != "" {
			return "cli"
		}
	case "cloudflare":
		tok, acc := cloudflareCreds()
		switch {
		case tok == "" || acc == "":
		case os.Getenv("CLOUDFLARE_API_TOKEN") != "":
			return "env"
		default:
			return "oryn"
		}
	}
	return ""
}

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

func (v *Vercel) Verify(ctx context.Context, token string) (string, string, error) {
	var r struct {
		User struct {
			Username string `json:"username"`
			Email    string `json:"email"`
		} `json:"user"`
	}
	if err := source.DoJSONH(ctx, "GET", v.base+"/v2/user", bearer(token), nil, &r); err != nil {
		return "", "", err
	}
	who := r.User.Username
	if who == "" {
		who = r.User.Email
	}
	return who, "", nil
}

func (v *Vercel) Projects(ctx context.Context) ([]ProjectRef, error) {
	tok := vercelToken()
	if tok == "" {
		return nil, ErrNoCredentials
	}
	var r struct {
		Projects []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			AccountID string `json:"accountId"`
		} `json:"projects"`
	}
	if err := source.DoJSONH(ctx, "GET", v.base+"/v9/projects?limit=100", bearer(tok), nil, &r); err != nil {
		return nil, err
	}
	out := make([]ProjectRef, 0, len(r.Projects))
	for _, p := range r.Projects {
		out = append(out, ProjectRef{Name: p.Name, Ref: p.ID, Team: p.AccountID})
	}
	return out, nil
}

func (n *Netlify) Verify(ctx context.Context, token string) (string, string, error) {
	var r struct {
		Name  string `json:"full_name"`
		Email string `json:"email"`
	}
	if err := source.DoJSONH(ctx, "GET", n.base+"/api/v1/user", bearer(token), nil, &r); err != nil {
		return "", "", err
	}
	who := r.Name
	if who == "" {
		who = r.Email
	}
	return who, "", nil
}

func (n *Netlify) Projects(ctx context.Context) ([]ProjectRef, error) {
	tok := netlifyToken()
	if tok == "" {
		return nil, ErrNoCredentials
	}
	var r []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := source.DoJSONH(ctx, "GET", n.base+"/api/v1/sites?per_page=100", bearer(tok), nil, &r); err != nil {
		return nil, err
	}
	out := make([]ProjectRef, 0, len(r))
	for _, s := range r {
		out = append(out, ProjectRef{Name: s.Name, Ref: s.ID})
	}
	return out, nil
}

// Verify lists accounts rather than hitting /user/tokens/verify, which only
// works for user-owned tokens and not account-owned ones.
func (c *Cloudflare) Verify(ctx context.Context, token string) (string, string, error) {
	var r struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := source.DoJSONH(ctx, "GET", c.base+"/client/v4/accounts", bearer(token), nil, &r); err != nil {
		return "", "", err
	}
	if len(r.Result) == 0 {
		return "", "", errors.New("token can't see any account (needs Account Settings: Read and Cloudflare Pages: Read)")
	}
	return r.Result[0].Name, r.Result[0].ID, nil
}

func (c *Cloudflare) Projects(ctx context.Context) ([]ProjectRef, error) {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return nil, ErrNoCredentials
	}
	var r struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	u := c.base + "/client/v4/accounts/" + url.PathEscape(acc) + "/pages/projects"
	if err := source.DoJSONH(ctx, "GET", u, bearer(tok), nil, &r); err != nil {
		return nil, err
	}
	out := make([]ProjectRef, 0, len(r.Result))
	for _, p := range r.Result {
		out = append(out, ProjectRef{Name: p.Name, Ref: p.Name})
	}
	return out, nil
}
