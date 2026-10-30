package deploy

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/Arnav717/oryn/internal/source"
)

// LogReader is a provider that can show a deployment's build log. Netlify
// isn't one: its API has no way to read a build log.
type LogReader interface {
	Logs(ctx context.Context, l Link, id string) ([]string, error)
}

// Rollbacker is a provider that can put an earlier deployment back live
// without rebuilding it.
type Rollbacker interface {
	Rollback(ctx context.Context, l Link, id string) error
}

func LogReaderFor(platform string) LogReader {
	r, _ := ProviderFor(platform).(LogReader)
	return r
}

func RollbackerFor(platform string) Rollbacker {
	r, _ := ProviderFor(platform).(Rollbacker)
	return r
}

func (v *Vercel) Logs(ctx context.Context, l Link, id string) ([]string, error) {
	tok := vercelToken()
	if tok == "" {
		return nil, ErrNoCredentials
	}
	q := url.Values{"builds": {"1"}, "limit": {"-1"}, "direction": {"forward"}}
	if strings.HasPrefix(l.Team, "team_") {
		q.Set("teamId", l.Team)
	}
	var events []struct {
		Text    string `json:"text"`
		Payload struct {
			Text string `json:"text"`
		} `json:"payload"`
	}
	u := v.base + "/v3/deployments/" + url.PathEscape(id) + "/events?" + q.Encode()
	if err := source.DoJSONH(ctx, http.MethodGet, u, map[string]string{"Authorization": "Bearer " + tok}, nil, &events); err != nil {
		return nil, err
	}
	var out []string
	for _, e := range events {
		text := e.Text
		if text == "" {
			text = e.Payload.Text
		}
		if text != "" {
			out = append(out, text)
		}
	}
	return out, nil
}

func (v *Vercel) Rollback(ctx context.Context, l Link, id string) error {
	tok := vercelToken()
	if tok == "" {
		return ErrNoCredentials
	}
	u := v.base + "/v1/projects/" + url.PathEscape(l.Ref) + "/rollback/" + url.PathEscape(id) + vercelTeamQuery(l)
	return source.DoJSONH(ctx, http.MethodPost, u, map[string]string{"Authorization": "Bearer " + tok}, nil, nil)
}

func (n *Netlify) Rollback(ctx context.Context, l Link, id string) error {
	tok := netlifyToken()
	if tok == "" {
		return ErrNoCredentials
	}
	u := n.base + "/api/v1/sites/" + url.PathEscape(l.Ref) + "/deploys/" + url.PathEscape(id) + "/restore"
	return source.DoJSONH(ctx, http.MethodPost, u, map[string]string{"Authorization": "Bearer " + tok}, nil, nil)
}

func (c *Cloudflare) deploymentURL(acc string, l Link, id string) string {
	return c.base + "/client/v4/accounts/" + url.PathEscape(acc) + "/pages/projects/" + url.PathEscape(l.Ref) + "/deployments/" + url.PathEscape(id)
}

func (c *Cloudflare) Logs(ctx context.Context, l Link, id string) ([]string, error) {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return nil, ErrNoCredentials
	}
	var r struct {
		Result struct {
			Data []struct {
				Line string `json:"line"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := source.DoJSONH(ctx, http.MethodGet, c.deploymentURL(acc, l, id)+"/history/logs", map[string]string{"Authorization": "Bearer " + tok}, nil, &r); err != nil {
		return nil, err
	}
	out := make([]string, len(r.Result.Data))
	for i, d := range r.Result.Data {
		out[i] = d.Line
	}
	return out, nil
}

func (c *Cloudflare) Rollback(ctx context.Context, l Link, id string) error {
	tok, acc := cloudflareCreds()
	if tok == "" || acc == "" {
		return ErrNoCredentials
	}
	return source.DoJSONH(ctx, http.MethodPost, c.deploymentURL(acc, l, id)+"/rollback", map[string]string{"Authorization": "Bearer " + tok}, nil, nil)
}
