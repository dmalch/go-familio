package familio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// tokenRe extracts the JWT bearer familio's Next.js SSR embeds in the page's
// __NEXT_DATA__ (`..."token":"eyJ..."...`). The cookie session bootstraps it;
// there is no token-mint API endpoint (see API.md "Auth — TWO-LAYER").
var tokenRe = regexp.MustCompile(`"token"\s*:\s*"(eyJ[A-Za-z0-9_\-.]+)"`)

// tokenSkew re-scrapes a little before the JWT's real expiry so a long apply
// never sends a token that expires mid-flight.
const tokenSkew = 5 * time.Minute

// bearerToken returns a valid JWT for the Authorization header, caching it until
// it nears expiry. It prefers the JWT the `t` session cookie already carries and
// falls back to scraping a familio.org HTML page. It also records the account
// uuid (userUUID), used as the ?owner= on person creates.
func (c *Client) bearerToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp.Add(-tokenSkew)) {
		return c.token, nil
	}

	// familio's `t` session cookie is itself the JWT the API wants, so when it
	// carries a usable one there is nothing to scrape.
	if token, exp, uuid, ok := c.sessionCookieToken(); ok {
		c.token = token
		c.tokenExp = exp
		c.userUUID = uuid
		return token, nil
	}

	token, err := c.scrapeToken(ctx)
	if err != nil {
		return "", err
	}

	exp, uuid, err := parseJWTClaims(token)
	if err != nil {
		// Token is opaque to us but still usable; just don't cache long.
		exp = time.Now().Add(tokenSkew)
	}
	c.token = token
	c.tokenExp = exp
	if uuid != "" {
		c.userUUID = uuid
	}
	return token, nil
}

// sessionCookieToken returns the JWT carried by the `t` session cookie, its
// expiry, and the account uuid — but only when the cookie really is a JWT with a
// uuid claim and enough life left to be worth caching. Anything else (an opaque
// session id, a malformed or near-expired token) reports ok=false so the caller
// falls back to the __NEXT_DATA__ scrape. That fallback is what keeps this safe
// if familio ever changes the cookie's contents.
func (c *Client) sessionCookieToken() (token string, exp time.Time, uuid string, ok bool) {
	if c.httpClient.Jar == nil {
		return "", time.Time{}, "", false
	}
	for _, cookie := range c.httpClient.Jar.Cookies(c.baseURL) {
		if cookie.Name != sessionCookieName {
			continue
		}
		value := cookie.Value
		// A header pasted from DevTools can carry the value percent-encoded.
		if unescaped, err := url.QueryUnescape(value); err == nil {
			value = unescaped
		}
		exp, uuid, err := parseJWTClaims(value)
		if err != nil || uuid == "" || !time.Now().Before(exp.Add(-tokenSkew)) {
			return "", time.Time{}, "", false
		}
		return value, exp, uuid, true
	}
	return "", time.Time{}, "", false
}

// scrapeToken fetches the familio.org landing page with the session cookie and
// pulls the embedded JWT out of __NEXT_DATA__.
func (c *Client) scrapeToken(ctx context.Context) (string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("familio: fetching auth token page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", ErrNotLoggedIn
	}

	// Only the first ~512 KB is needed; __NEXT_DATA__ sits in the document head/tail.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("familio: reading auth token page: %w", err)
	}
	m := tokenRe.FindSubmatch(body)
	if m == nil {
		// No embedded token ⇒ the page rendered logged-out.
		return "", ErrNotLoggedIn
	}
	return string(m[1]), nil
}

// jwtClaims is the subset of the familio JWT payload the client needs.
type jwtClaims struct {
	Exp  int64  `json:"exp"`
	UUID string `json:"uuid"`
}

// parseJWTClaims decodes (without verifying — we don't hold the RS256 key) the
// JWT payload to read its expiry and the account uuid.
func parseJWTClaims(token string) (time.Time, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, "", fmt.Errorf("familio: malformed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("familio: decoding JWT payload: %w", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, "", fmt.Errorf("familio: parsing JWT payload: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, claims.UUID, fmt.Errorf("familio: JWT has no exp")
	}
	return time.Unix(claims.Exp, 0), claims.UUID, nil
}
