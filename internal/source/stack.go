package source

import (
	"sort"
	"strings"

	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/detect"
)

// vendor maps dependency names to a statuspage-compatible status page.
// Patterns are "<ecosystem>:<name>"; a trailing * matches any suffix.
// Only pages serving /api/v2/status.json belong here.
type vendor struct {
	Name string
	URL  string
	Pkgs []string
}

var vendors = []vendor{
	{"Twilio", "https://status.twilio.com", []string{"npm:twilio", "PyPI:twilio", "Go:github.com/twilio/*", "crates.io:twilio"}},
	{"Sentry", "https://status.sentry.io", []string{"npm:@sentry/*", "PyPI:sentry-sdk", "Go:github.com/getsentry/*", "crates.io:sentry"}},
	{"Mailgun", "https://status.mailgun.com", []string{"npm:mailgun.js", "npm:mailgun-js", "PyPI:mailgun", "Go:github.com/mailgun/*"}},
	{"Notion", "https://www.notion-status.com", []string{"npm:@notionhq/*", "PyPI:notion-client", "Go:github.com/jomei/notionapi"}},
	{"Figma", "https://status.figma.com", []string{"npm:figma-api", "npm:@figma/*"}},
	{"Mapbox", "https://status.mapbox.com", []string{"npm:mapbox-gl", "npm:@mapbox/*", "PyPI:mapbox"}},
	{"Segment", "https://status.segment.com", []string{"npm:@segment/*", "npm:analytics-node", "PyPI:segment-analytics-python", "Go:github.com/segmentio/analytics-go*"}},
	{"HashiCorp", "https://status.hashicorp.com", []string{"npm:@hashicorp/*", "Go:github.com/hashicorp/vault*", "Go:github.com/hashicorp/consul*", "Go:github.com/hashicorp/terraform*"}},
	{"Elastic", "https://status.elastic.co", []string{"npm:@elastic/*", "PyPI:elasticsearch", "Go:github.com/elastic/*", "crates.io:elasticsearch"}},
	{"Pusher", "https://status.pusher.com", []string{"npm:pusher", "npm:pusher-js", "PyPI:pusher", "Go:github.com/pusher/*"}},
	{"Ably", "https://status.ably.com", []string{"npm:ably", "PyPI:ably", "Go:github.com/ably/*"}},
	{"Pinecone", "https://status.pinecone.io", []string{"npm:@pinecone-database/*", "PyPI:pinecone", "PyPI:pinecone-client"}},
	{"MongoDB", "https://status.mongodb.com", []string{"npm:mongodb", "npm:mongoose", "PyPI:pymongo", "PyPI:motor", "Go:go.mongodb.org/*", "crates.io:mongodb"}},
	{"LaunchDarkly", "https://status.launchdarkly.com", []string{"npm:launchdarkly-*", "npm:@launchdarkly/*", "PyPI:launchdarkly-server-sdk", "Go:github.com/launchdarkly/*"}},
	{"Contentful", "https://www.contentfulstatus.com", []string{"npm:contentful", "npm:contentful-*", "PyPI:contentful"}},
	{"Supabase", "https://status.supabase.com", []string{"npm:@supabase/*", "PyPI:supabase", "Go:github.com/supabase-community/*"}},
	{"Vercel", "https://www.vercel-status.com", []string{"npm:vercel", "npm:@vercel/*", "npm:next"}},
	{"Netlify", "https://www.netlifystatus.com", []string{"npm:netlify-cli", "npm:@netlify/*"}},
	{"Cloudflare", "https://www.cloudflarestatus.com", []string{"npm:wrangler", "npm:@cloudflare/*", "PyPI:cloudflare", "Go:github.com/cloudflare/*"}},
	{"OpenAI", "https://status.openai.com", []string{"npm:openai", "PyPI:openai", "Go:github.com/openai/*", "Go:github.com/sashabaranov/go-openai"}},
	{"Claude", "https://status.claude.com", []string{"npm:@anthropic-ai/*", "PyPI:anthropic", "Go:github.com/anthropics/*", "crates.io:anthropic*"}},
	{"Discord", "https://discordstatus.com", []string{"npm:discord.js", "npm:discord-api-types", "PyPI:discord.py", "Go:github.com/bwmarrin/discordgo"}},
}

func (v vendor) matches(d detect.Dep) bool {
	key := strings.ToLower(d.Ecosystem + ":" + d.Name)
	for _, p := range v.Pkgs {
		if pre, ok := strings.CutSuffix(p, "*"); ok {
			if strings.HasPrefix(key, strings.ToLower(pre)) {
				return true
			}
		} else if key == strings.ToLower(p) {
			return true
		}
	}
	return false
}

// StackServices returns the vendor status pages implied by deps. Via names the
// first direct dependency that matched. Direct deps win over transitive ones.
func StackServices(deps []detect.Dep) []config.Service {
	via := map[int]string{}
	for i, v := range vendors {
		for _, d := range deps {
			if !d.Direct || !v.matches(d) {
				continue
			}
			if cur, ok := via[i]; !ok || d.Name < cur {
				via[i] = d.Name
			}
		}
	}
	idx := make([]int, 0, len(via))
	for i := range via {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	out := make([]config.Service, 0, len(idx))
	for _, i := range idx {
		out = append(out, config.Service{Name: vendors[i].Name, URL: vendors[i].URL, Via: via[i]})
	}
	return out
}

// MergeServices appends extra to base, skipping URLs already present. A base
// entry matching an extra one inherits its Via so the UI can say why.
func MergeServices(base, extra []config.Service) []config.Service {
	norm := func(u string) string { return strings.ToLower(strings.TrimRight(u, "/")) }
	out := append([]config.Service(nil), base...)
	pos := map[string]int{}
	for i, s := range out {
		pos[norm(s.URL)] = i
	}
	for _, s := range extra {
		if i, ok := pos[norm(s.URL)]; ok {
			if out[i].Via == "" {
				out[i].Via = s.Via
			}
			continue
		}
		out = append(out, s)
	}
	return out
}
