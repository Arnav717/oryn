package deploy

import (
	"context"
	"net/url"
	"strings"

	"github.com/Arnav717/oryn/internal/source"
)

// Remover deletes deployments and projects on the platform. These are
// destructive and irreversible; callers must confirm with the user first.
type Remover interface {
	DeleteDeployment(ctx context.Context, l Link, id string) error
	DeleteProject(ctx context.Context, l Link) error
}

// RemoverFor returns the platform's Remover, or nil.
func RemoverFor(platform string) Remover {
	r, _ := ConnectorFor(platform).(Remover)
	return r
}

func vercelTeamQuery(l Link) string {
	if strings.HasPrefix(l.Team, "team_") {
		return "?teamId=" + url.QueryEscape(l.Team)
	}
	return ""
}

func (v *Vercel) DeleteDeployment(ctx context.Context, l Link, id string) error {
	tok := vercelToken()
	if tok == "" {
		return ErrNoCredentials
	}
	return source.DoJSONH(ctx, "DELETE", v.base+"/v13/deployments/"+url.PathEscape(id)+vercelTeamQuery(l), bearer(tok), nil, nil)
}

func (v *Vercel) DeleteProject(ctx context.Context, l Link) error {
	tok := vercelToken()
	if tok == "" {
		return ErrNoCredentials
	}
	return source.DoJSONH(ctx, "DELETE", v.base+"/v9/projects/"+url.PathEscape(l.Ref)+vercelTeamQuery(l), bearer(tok), nil, nil)
}

func (n *Netlify) DeleteDeployment(ctx context.Context, l Link, id string) error {
	tok := netlifyToken()
	if tok == "" {
		return ErrNoCredentials
	}
	return source.DoJSONH(ctx, "DELETE", n.base+"/api/v1/deploys/"+url.PathEscape(id), bearer(tok), nil, nil)
}

func (n *Netlify) DeleteProject(ctx context.Context, l Link) error {
	tok := netlifyToken()
	if tok == "" {
		return ErrNoCredentials
	}
	return source.DoJSONH(ctx, "DELETE", n.base+"/api/v1/sites/"+url.PathEscape(l.Ref), bearer(tok), nil, nil)
}

func (c *Cloudflare) DeleteDeployment(ctx context.Context, l Link, id string) error {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return ErrNoCredentials
	}
	// Cloudflare refuses a deployment with a branch alias unless forced. The
	// delete is already confirmed, and force doesn't cover the active production
	// deployment, which it always refuses.
	u := c.base + "/client/v4/accounts/" + url.PathEscape(acc) + "/pages/projects/" + url.PathEscape(l.Ref) + "/deployments/" + url.PathEscape(id) + "?force=true"
	return source.DoJSONH(ctx, "DELETE", u, bearer(tok), nil, nil)
}

func (c *Cloudflare) DeleteProject(ctx context.Context, l Link) error {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return ErrNoCredentials
	}
	u := c.base + "/client/v4/accounts/" + url.PathEscape(acc) + "/pages/projects/" + url.PathEscape(l.Ref)
	return source.DoJSONH(ctx, "DELETE", u, bearer(tok), nil, nil)
}
