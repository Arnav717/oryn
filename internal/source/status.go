package source

import (
	"context"
	"strings"
	"time"

	"github.com/Arnav717/oryn/internal/config"
)

type Status struct {
	Service     string
	URL         string
	Indicator   string // none, minor, major, critical, maintenance; "" on error
	Description string
	Err         error
	Checked     time.Time
}

func (s Status) OK() bool { return s.Err == nil && s.Indicator == "none" }

// CheckStatus polls each service's statuspage-compatible /api/v2/status.json.
func CheckStatus(ctx context.Context, services []config.Service) []Status {
	out := make([]Status, len(services))
	idx := make([]int, len(services))
	for i := range idx {
		idx[i] = i
	}
	forEach(idx, 8, func(i int) {
		svc := services[i]
		var r struct {
			Status struct {
				Indicator   string `json:"indicator"`
				Description string `json:"description"`
			} `json:"status"`
		}
		err := getJSON(ctx, strings.TrimRight(svc.URL, "/")+"/api/v2/status.json", &r)
		out[i] = Status{
			Service:     svc.Name,
			URL:         svc.URL,
			Indicator:   r.Status.Indicator,
			Description: r.Status.Description,
			Err:         err,
			Checked:     time.Now(),
		}
	})
	return out
}
