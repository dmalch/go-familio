// Package familio is a Go client for the familio.org genealogy API.
//
// It covers the account profile, the tree (both the one-request graph and a
// bounded crawl), person CRUD, life-fact events — which is where familio keeps
// kinship, including marriages — biographies, source citations, settlements, and
// the change history, matches, and tags features. See API.md for the endpoint
// reference and Client for the auth model.
//
// The domain is event-centric: familio has no relationship resource, so a
// marriage is a wedding event with two spouse participants and a parent-child
// link is a birth event. DeriveRelations normalizes that into
// parents/spouses/children.
//
// Errors: every response >= 400 is an *APIError carrying the status, and wraps
// ErrNotFound, ErrNotLoggedIn, ErrAccessDenied, or ErrConflict where the status
// maps to one — so both errors.Is and errors.As work.
//
// This is an unofficial integration: familio.org publishes no write API and
// these endpoints were reverse-engineered. See the README's Stability section.
package familio

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Version is this module's version, sent in the default User-Agent. Bump it in
// the same commit as the release tag (see CONTRIBUTING.md).
const Version = "1.2.0"

const (
	defaultBaseURL   = "https://familio.org/"
	apiV2Path        = "api/v2/"
	apiV3Path        = "api/v3/" // the person search; everything else is v2
	defaultUserAgent = "go-familio/" + Version + " (+https://github.com/dmalch/go-familio)"
	defaultRateLimit = 2.0
	defaultTimeout   = 60 * time.Second
)

// Client talks to familio.org's /api/v2 surface with a session cookie.
//
// Authenticated calls need a JWT bearer, which familio does not mint via an API
// endpoint. The client obtains one on first use and refreshes it near expiry,
// preferring the JWT the `t` session cookie already carries and falling back to
// scraping the page's __NEXT_DATA__; see auth.go. A Client is safe for
// concurrent use.
type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
	userAgent  string
	limiter    *rate.Limiter

	// token cache (guarded by mu)
	mu       sync.Mutex
	token    string
	tokenExp time.Time
	userUUID string // current account uuid, from the JWT — used as ?owner=
}

// Options configures a Client. At least one cookie carrying the `t` session
// token should be supplied for any authenticated (write, or gated read) call;
// the public settlement-persons read works without one.
type Options struct {
	// Cookies carries the familio.org session. NewClient installs them on a
	// cookie jar scoped to BaseURL. Build with CookiesFromHeader (paste from
	// DevTools / $FAMILIO_COOKIES) or CookiesFromBrowser (sweetcookie).
	Cookies []*http.Cookie

	// BaseURL overrides https://familio.org/. Useful for tests; production
	// callers should leave it empty.
	BaseURL string

	// UserAgent is sent on every request. Defaults to defaultUserAgent.
	UserAgent string

	// RateLimit caps outgoing requests in requests-per-second. Defaults to
	// defaultRateLimit if unset or non-positive.
	RateLimit float64

	// HTTPClient overrides the default *http.Client. NewClient always sets its
	// Jar from Cookies and its CheckRedirect to detect login bounces, so the
	// override does not need to.
	HTTPClient *http.Client
}

// NewClient builds a Client from Options.
func NewClient(opts Options) (*Client, error) {
	rawBase := opts.BaseURL
	if rawBase == "" {
		rawBase = defaultBaseURL
	}
	base, err := url.Parse(rawBase)
	if err != nil {
		return nil, fmt.Errorf("familio: invalid base URL %q: %w", rawBase, err)
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("familio: building cookie jar: %w", err)
	}
	if len(opts.Cookies) > 0 {
		jar.SetCookies(base, opts.Cookies)
	}
	httpClient.Jar = jar

	// A redirect to a login/auth path means the session is not valid. Surface
	// it as ErrNotLoggedIn instead of silently following to an HTML login form.
	httpClient.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if isLoginPath(req.URL.Path) {
			return ErrNotLoggedIn
		}
		return nil
	}

	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	rl := opts.RateLimit
	if rl <= 0 {
		rl = defaultRateLimit
	}

	return &Client{
		httpClient: httpClient,
		baseURL:    base,
		userAgent:  userAgent,
		limiter:    rate.NewLimiter(rate.Limit(rl), 1),
	}, nil
}

func isLoginPath(path string) bool {
	path = strings.ToLower(path)
	return strings.Contains(path, "/login") || strings.Contains(path, "/auth/") || strings.Contains(path, "/signin")
}
