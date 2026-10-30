// Package source fetches data from external services (OSV, registries, status pages).
package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 20 * time.Second}

const userAgent = "oryn/0.1 (+https://github.com/Arnav717/oryn)"

func getJSON(ctx context.Context, url string, out any) error {
	return doJSON(ctx, http.MethodGet, url, nil, out)
}

func doJSON(ctx context.Context, method, url string, body, out any) error {
	return doJSONH(ctx, method, url, nil, body, out)
}

// doJSONH is doJSON with extra request headers (e.g. Authorization). Headers
// are never included in returned errors.
func doJSONH(ctx context.Context, method, url string, headers map[string]string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		// Writes get a fresh connection: Go can't safely retry a POST on an
		// idle connection the server already closed, which shows up as EOF.
		req.Close = true
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if msg := apiMessage(resp.Body); msg != "" {
			return fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, msg)
		}
		return fmt.Errorf("%s %s: %s", method, url, resp.Status)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// apiMessage pulls a short human message out of a JSON error body, the shapes
// Vercel ({"error":{"message"}}), Cloudflare ({"errors":[{"message"}]}) and
// Netlify ({"message"}) use. Anything else yields "".
func apiMessage(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 4096))
	var v struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	msg := v.Message
	if msg == "" {
		msg = v.Error.Message
	}
	if msg == "" && len(v.Errors) > 0 {
		msg = v.Errors[0].Message
	}
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return msg
}

// forEach runs fn over items with at most n concurrent calls.
func forEach[T any](items []T, n int, fn func(T)) {
	sem := make(chan struct{}, n)
	done := make(chan struct{})
	for _, it := range items {
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; done <- struct{}{} }()
			fn(it)
		}()
	}
	for range items {
		<-done
	}
}

func getText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(b), err
}

// DoJSONH is the exported form of doJSONH for packages that call
// authenticated APIs. Headers never appear in returned errors.
func DoJSONH(ctx context.Context, method, url string, headers map[string]string, body, out any) error {
	return doJSONH(ctx, method, url, headers, body, out)
}
