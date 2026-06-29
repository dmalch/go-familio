package familio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const maxAttempts = 3

// newRequest builds a request against BaseURL + api/v2/ + path with standard
// headers. body, when non-nil, is JSON-encoded.
func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	rel := &url.URL{Path: apiV2Path + path}
	u := c.baseURL.ResolveReference(rel)
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("familio: encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// newAuthedRequest builds a request like newRequest but for the authenticated,
// API-Platform/Hydra tree-editor endpoints: it negotiates application/ld+json
// and attaches the scraped JWT bearer (fetching one if needed).
func (c *Client) newAuthedRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	req, err := c.newRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/ld+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/ld+json")
	}
	token, err := c.bearerToken(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

// do executes req (honoring the rate limiter, with a small retry on 429/5xx)
// and decodes a JSON response into out (out may be nil to discard the body).
func (c *Client) do(req *http.Request, out any) error {
	if err := c.limiter.Wait(req.Context()); err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Reset the body for retries of requests that carry one.
		if attempt > 1 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return err
			}
			req.Body = body
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// A CheckRedirect sentinel (ErrNotLoggedIn) arrives wrapped in a
			// *url.Error — unwrap and surface it directly.
			if errors.Is(err, ErrNotLoggedIn) {
				return ErrNotLoggedIn
			}
			lastErr = err
			if attempt < maxAttempts {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return err
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("familio: reading response body: %w", readErr)
		}

		switch {
		case resp.StatusCode == http.StatusNotFound:
			return ErrNotFound
		case resp.StatusCode == http.StatusUnauthorized:
			return ErrNotLoggedIn
		case resp.StatusCode == http.StatusForbidden:
			return ErrAccessDenied
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("familio: %s %s: HTTP %d: %s",
				req.Method, req.URL.Path, resp.StatusCode, snippet(body))
			if attempt < maxAttempts {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return lastErr
		case resp.StatusCode >= 400:
			return fmt.Errorf("familio: %s %s: HTTP %d: %s",
				req.Method, req.URL.Path, resp.StatusCode, snippet(body))
		}

		if out == nil || len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("familio: decoding response: %w (body: %s)", err, snippet(body))
		}
		return nil
	}
	return lastErr
}

func retryBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * time.Second
}

func snippet(b []byte) string {
	const limit = 300
	if len(b) > limit {
		return string(b[:limit]) + "…"
	}
	return string(b)
}
