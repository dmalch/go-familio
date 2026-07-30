package familio

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	. "github.com/onsi/gomega"
)

// testOwnerUUID is the account uuid embedded in the test JWT. The owner-addressed
// collections (history, matches, tags) build it into their paths via AccountUUID.
const testOwnerUUID = "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0"

// testJWT builds an unsigned JWT whose payload carries a far-future exp and the
// testOwnerUUID uuid claim, so AccountUUID resolves without a network round-trip
// beyond the token scrape.
func testJWT() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"exp":9999999999,"uuid":"` + testOwnerUUID + `"}`))
	return header + "." + payload + ".sig"
}

// authedTestServer serves the __NEXT_DATA__ token page on / — so the client's
// bearer scrape succeeds and yields testOwnerUUID — and delegates every other
// path to handler. Use it for any endpoint that needs a bearer.
func authedTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"`+testJWT()+`"}}</script>`)
			return
		}
		handler(w, r)
	}))
}

// newTestClient builds a client aimed at srv with a session cookie and the rate
// limiter effectively disabled.
func newTestClient(srv *httptest.Server) *Client {
	c, _ := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookiesFromHeader("t=secret"),
		RateLimit: 1000,
	})
	return c
}

// newLiveClient builds a client from the ambient credentials, skipping the test
// when none are configured. Used by the FAMILIO_NETWORK_TEST live decode tests.
func newLiveClient(t *testing.T) *Client {
	t.Helper()
	switch {
	case os.Getenv("FAMILIO_COOKIES") != "":
		c, err := NewClient(Options{Cookies: CookiesFromHeader(os.Getenv("FAMILIO_COOKIES"))})
		Expect(err).ToNot(HaveOccurred())
		return c
	case os.Getenv("FAMILIO_SESSION") != "":
		c, err := NewClient(Options{Cookies: CookieFromSessionToken(os.Getenv("FAMILIO_SESSION"))})
		Expect(err).ToNot(HaveOccurred())
		return c
	case os.Getenv("FAMILIO_BROWSER") != "":
		cookies, err := CookiesFromBrowser(os.Getenv("FAMILIO_BROWSER"))
		Expect(err).ToNot(HaveOccurred())
		c, err := NewClient(Options{Cookies: cookies})
		Expect(err).ToNot(HaveOccurred())
		return c
	default:
		t.Skip("set FAMILIO_COOKIES, FAMILIO_SESSION, or FAMILIO_BROWSER to run the live test")
		return nil
	}
}

// asMap asserts that a decoded JSON value is an object, so tests can walk a
// request body without unchecked type assertions.
func asMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	Expect(ok).To(BeTrue(), "expected a JSON object, got %T", v)
	return m
}

// asSlice asserts that a decoded JSON value is an array.
func asSlice(v any) []any {
	s, ok := v.([]any)
	Expect(ok).To(BeTrue(), "expected a JSON array, got %T", v)
	return s
}
