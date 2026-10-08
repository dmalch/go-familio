package familio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// RawResponse is familio.org's answer to DoRaw, undecoded.
type RawResponse struct {
	// StatusCode is the final status, after any retries.
	StatusCode int

	// Header is the final response's headers.
	Header http.Header

	// Body is the whole response body — not the few hundred bytes an APIError
	// keeps.
	Body []byte
}

// DoRaw sends an arbitrary request to familio.org's API — the escape hatch for an
// endpoint this package does not model. It goes through the same machinery as
// every typed call: the rate limiter, the retry on 429 and 5xx, the User-Agent,
// and the JWT bearer.
//
// The bearer is attached when the client holds a `t` session cookie. Without one
// the request goes out anonymously, which only the public reads (the settlement
// list, the person search) accept — and no token scrape is attempted.
//
// endpoint names the resource relative to the /api/v2 root ("profile",
// "persons/<uuid>/events?…"), rooted ("/api/v2/profile", as familio prints its
// own links), or as a full URL under BaseURL's api/. Naming another version
// keeps it: "/api/v3/persons?…" is the v3 person search. A URL anywhere else is
// refused before any request is made, so the bearer never leaves familio.org.
//
// Accept defaults to application/ld+json, and so does Content-Type when body is
// non-nil — what the typed calls send. header is applied last, so it can override
// either. body may be nil.
//
// Unlike the typed methods, a response status >= 400 is not an error: it comes
// back as the RawResponse, with the server's whole body, for the caller to show.
// The error is for a refused endpoint, a session that yields no bearer or
// redirects to a login page (ErrNotLoggedIn), and transport failures.
func (c *Client) DoRaw(ctx context.Context, method, endpoint string, header http.Header, body []byte) (*RawResponse, error) {
	u, err := c.rawURL(endpoint)
	if err != nil {
		return nil, err
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/ld+json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/ld+json")
	}
	if err := c.addOptionalBearer(ctx, req); err != nil {
		return nil, err
	}
	for name, values := range header {
		req.Header.Del(name)
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}

	return c.send(req)
}

// apiVersionRe matches the version an endpoint names, as in "v3/persons" after
// its "api/" prefix.
var apiVersionRe = regexp.MustCompile(`^v[0-9]+/`)

// rawURL resolves a DoRaw endpoint against the API root, refusing anything that
// would leave it. An endpoint that names a version ("/api/v3/persons") keeps
// it; one that does not is under api/v2/.
func (c *Client) rawURL(endpoint string) (*url.URL, error) {
	root := c.baseURL.ResolveReference(&url.URL{Path: "api/"})

	var rest string // relative to root, starting with the version
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		var ok bool
		rest, ok = strings.CutPrefix(endpoint, root.String())
		if !ok || !apiVersionRe.MatchString(rest) {
			return nil, fmt.Errorf("familio: %s is not a familio.org API URL (%s…)", endpoint, root)
		}
	} else {
		rest = strings.TrimPrefix(endpoint, "/")
		if versioned, ok := strings.CutPrefix(rest, "api/"); ok && apiVersionRe.MatchString(versioned) {
			rest = versioned
		} else if !apiVersionRe.MatchString(rest) {
			rest = strings.TrimPrefix(apiV2Path, "api/") + rest
		}
	}
	version := apiVersionRe.FindString(rest)
	if path, _, _ := strings.Cut(strings.TrimPrefix(rest, version), "?"); strings.Trim(path, "/") == "" {
		return nil, fmt.Errorf("familio: no endpoint in %q", endpoint)
	}

	u, err := url.Parse(root.String() + rest)
	if err != nil {
		return nil, fmt.Errorf("familio: invalid endpoint %q: %w", endpoint, err)
	}
	if u.Scheme != root.Scheme || u.Host != root.Host {
		return nil, fmt.Errorf("familio: %s is not a familio.org API URL (%s…)", endpoint, root)
	}
	return u, nil
}

// addOptionalBearer attaches the bearer when the client holds a `t` session
// cookie and leaves the request anonymous otherwise, for the endpoints that
// answer both ways: with no cookie no token scrape is attempted. The check is
// for `t` itself because the jar also collects familio's DataDome cookies.
func (c *Client) addOptionalBearer(ctx context.Context, req *http.Request) error {
	if !c.hasSessionCookie() {
		return nil
	}
	token, err := c.bearerToken(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// hasSessionCookie reports whether the client holds a `t` session cookie — the
// only thing a bearer can be obtained from.
func (c *Client) hasSessionCookie() bool {
	if c.httpClient.Jar == nil {
		return false
	}
	for _, cookie := range c.httpClient.Jar.Cookies(c.baseURL) {
		if cookie.Name == sessionCookieName {
			return true
		}
	}
	return false
}
