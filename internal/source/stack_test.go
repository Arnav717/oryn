package source

import (
	"testing"

	"github.com/Arnav717/oryn/internal/config"
	"github.com/Arnav717/oryn/internal/detect"
)

func TestStackServices(t *testing.T) {
	deps := []detect.Dep{
		{Ecosystem: detect.NPM, Name: "@sentry/node", Direct: true},
		{Ecosystem: detect.NPM, Name: "twilio", Direct: false}, // transitive: ignored
		{Ecosystem: detect.PyPI, Name: "OpenAI", Direct: true},
		{Ecosystem: detect.Go, Name: "github.com/twilio/twilio-go", Direct: true},
	}
	got := StackServices(deps)
	want := map[string]string{"Sentry": "@sentry/node", "OpenAI": "OpenAI", "Twilio": "github.com/twilio/twilio-go"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, s := range got {
		if want[s.Name] != s.Via {
			t.Errorf("%s via %q, want %q", s.Name, s.Via, want[s.Name])
		}
	}
}

func TestMergeServices(t *testing.T) {
	base := []config.Service{{Name: "Sentry", URL: "https://status.sentry.io/"}}
	extra := []config.Service{{Name: "Sentry", URL: "https://status.sentry.io", Via: "@sentry/node"}, {Name: "Twilio", URL: "https://status.twilio.com", Via: "twilio"}}
	got := MergeServices(base, extra)
	if len(got) != 2 || got[0].Via != "@sentry/node" || got[1].Name != "Twilio" {
		t.Fatalf("got %+v", got)
	}
}
