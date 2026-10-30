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
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s", method, url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
