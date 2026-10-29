package browser

import "testing"

func TestOpenRefusesNonHTTPS(t *testing.T) {
	for _, u := range []string{"http://example.com", "file:///etc/passwd", "javascript:alert(1)", "--help", ""} {
		if err := Open(u); err == nil {
			t.Errorf("Open(%q) succeeded, want refusal", u)
		}
	}
}
