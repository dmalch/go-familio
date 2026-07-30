package familio

import (
	"net/http"
	"net/url"
	"strings"
)

// sessionCookieName is familio's session cookie. Its value is itself the JWT the
// authed API wants (see auth.go).
const sessionCookieName = "t"

// CookiesFromHeader parses a "name=value; name=value" cookie header (the form
// copied out of a browser's DevTools Network panel, or the $FAMILIO_COOKIES env
// var) into a slice of *http.Cookie suitable for Options.Cookies. Lifted from
// go-geni's web.CookiesFromHeader.
func CookiesFromHeader(header string) []*http.Cookie {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	pairs := strings.Split(header, ";")
	cookies := make([]*http.Cookie, 0, len(pairs))
	for _, p := range pairs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		if eq <= 0 {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:  p[:eq],
			Value: encodeCookieValue(p[eq+1:]),
		})
	}
	return cookies
}

// CookieFromSessionToken wraps a bare `t` session token value (from the
// session_token provider attr / $FAMILIO_SESSION) as a single `t` cookie.
func CookieFromSessionToken(token string) []*http.Cookie {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return []*http.Cookie{{Name: sessionCookieName, Value: encodeCookieValue(token)}}
}

// encodeCookieValue percent-encodes a value that net/http would otherwise refuse
// to send. familio's `t` cookie holds a JSON object, and net/http silently drops
// bytes that are illegal in a cookie value (notably `"`), which mangles the
// credential — the browser sends it percent-encoded, so match that. A value that
// is already legal (an encoded one included) is returned untouched, so this never
// double-encodes.
func encodeCookieValue(value string) string {
	if isValidCookieValue(value) {
		return value
	}
	return url.QueryEscape(value)
}

// isValidCookieValue reports whether every byte is legal in a cookie value, per
// the set net/http enforces when writing the header.
func isValidCookieValue(value string) bool {
	for i := range len(value) {
		b := value[i]
		switch {
		case b == '"', b == ';', b == '\\', b == ',', b == ' ':
			return false
		case b < 0x21 || b > 0x7e:
			return false
		}
	}
	return true
}
