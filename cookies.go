package familio

import (
	"net/http"
	"strings"
)

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
			Value: p[eq+1:],
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
	return []*http.Cookie{{Name: "t", Value: token}}
}
