package deploy

import (
	"context"
	"errors"
	"time"
)

// Deployment states, normalised across platforms.
const (
	StateReady    = "ready"
	StateBuilding = "building"
	StateQueued   = "queued"
	StateError    = "error"
	StateCanceled = "canceled"
)

type Deployment struct {
	ID       string
	Platform string
	Target   string // production, preview
	State    string
	Branch   string
	Commit   string // short sha
	Message  string
	URL      string
	Created  time.Time
}

// ErrNoCredentials means no token was found for the platform; callers show
// how to log in rather than treating it as a failure.
var ErrNoCredentials = errors.New("no credentials")

// Provider reads deployments for a linked project.
type Provider interface {
	Name() string
	List(ctx context.Context, l Link, limit int) ([]Deployment, error)
}

// ProviderFor returns the provider for a platform, or nil when oryn can
// detect the platform but cannot query it yet.
func ProviderFor(platform string) Provider {
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

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
