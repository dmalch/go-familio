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

// sessionEnvelope is the JSON object familio's `t` cookie actually holds. The
// bearer is the token field; synapseToken belongs to familio's chat and is not a
// credential for /api/v2.
type sessionEnvelope struct {
	Token string `json:"token"`
}

// sessionCookieToken returns the JWT carried by the `t` session cookie, its
// expiry, and the account uuid — but only when the cookie yields a well-formed
// JWT with a uuid claim and enough life left to be worth caching. Anything else
// (an opaque session id, an envelope without a token, a malformed or
// near-expired JWT) reports ok=false so the caller falls back to the
// __NEXT_DATA__ scrape. That fallback is what keeps this working if familio
// changes the cookie's contents.
func (c *Client) sessionCookieToken() (token string, exp time.Time, uuid string, ok bool) {
	if c.httpClient.Jar == nil {
		return "", time.Time{}, "", false
	}
	for _, cookie := range c.httpClient.Jar.Cookies(c.baseURL) {
		if cookie.Name != sessionCookieName {
			continue
		}
		candidate, found := jwtFromCookieValue(cookie.Value)
		if !found {
			return "", time.Time{}, "", false
		}
		exp, uuid, err := parseJWTClaims(candidate)
		if err != nil || uuid == "" || !time.Now().Before(exp.Add(-tokenSkew)) {
			return "", time.Time{}, "", false
		}
		return candidate, exp, uuid, true
	}
	return "", time.Time{}, "", false
}

// jwtFromCookieValue extracts the bearer from a `t` cookie value. familio stores
// a JSON envelope (`{"token":"eyJ…","synapseToken":"syt_…"}`), usually
// percent-encoded on the wire; a bare JWT is accepted too, since that is what
// callers who only kept the token will pass.
//
// The envelope must be parsed as JSON, not sniffed: its inner JWT contributes
// exactly two dots, so splitting the raw envelope on "." yields three parts and a
// naive parser will happily decode the middle one and then send the whole
// envelope as the bearer (familio answers 401 «Invalid JWT Token»).
func jwtFromCookieValue(value string) (string, bool) {
	if unescaped, err := url.QueryUnescape(value); err == nil {
		value = unescaped
	}
	value = strings.TrimSpace(value)

	if strings.HasPrefix(value, "{") {
		var envelope sessionEnvelope
		if err := json.Unmarshal([]byte(value), &envelope); err != nil {
			return "", false
		}
		return envelope.Token, looksLikeJWT(envelope.Token)
	}
	return value, looksLikeJWT(value)
}

// looksLikeJWT reports whether s is shaped like a JWT: three segments whose
// header decodes as base64url JSON. It rejects the values that must fall through
// to the scrape before they are sent as a bearer.
func looksLikeJWT(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	return err == nil && strings.HasPrefix(string(header), "{")
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
