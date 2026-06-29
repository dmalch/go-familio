package familio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// bearerToken returns a valid JWT for the Authorization header, scraping (and
// caching) it from a familio.org HTML page when absent or near expiry. It also
// records the account uuid (userUUID), used as the ?owner= on person creates.
func (c *Client) bearerToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp.Add(-tokenSkew)) {
		return c.token, nil
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
