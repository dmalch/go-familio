package familio

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	. "github.com/onsi/gomega"
)

// jwtWithExp builds an unsigned JWT carrying the given exp and uuid claims.
func jwtWithExp(exp int64, uuid string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"exp":` + strconv.FormatInt(exp, 10) + `,"uuid":"` + uuid + `"}`))
	return header + "." + payload + ".sig"
}

// TestBearerFromSessionCookie proves the fast path: the `t` cookie value is
// itself a usable JWT, so no HTML page has to be fetched and regexed to obtain
// the bearer.
func TestBearerFromSessionCookie(t *testing.T) {
	RegisterTestingT(t)
	var scrapes, apiCalls int32
	cookieJWT := jwtWithExp(9999999999, testOwnerUUID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			atomic.AddInt32(&scrapes, 1)
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"`+testJWT()+`"}}</script>`)
			return
		}
		atomic.AddInt32(&apiCalls, 1)
		Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + cookieJWT))
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	client, err := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookieFromSessionToken(cookieJWT),
		RateLimit: 1000,
	})
	Expect(err).ToNot(HaveOccurred())

	uuid, err := client.AccountUUID(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(uuid).To(Equal(testOwnerUUID))

	_, err = client.ListTags(context.Background())
	Expect(err).ToNot(HaveOccurred())

	Expect(apiCalls).To(Equal(int32(1)))
	Expect(scrapes).To(BeZero(), "the cookie already carried a usable JWT — no page fetch should happen")
}

// TestBearerFromJSONSessionCookie covers the shape a real browser session has:
// familio's `t` cookie is not a bare JWT but a JSON envelope carrying the JWT
// plus a Matrix/Synapse token. Only the "token" field may be used as the bearer —
// sending the whole envelope earns a 401 "Invalid JWT Token".
func TestBearerFromJSONSessionCookie(t *testing.T) {
	RegisterTestingT(t)
	var scrapes int32
	inner := jwtWithExp(9999999999, testOwnerUUID)
	envelope := `{"token":"` + inner + `","synapseToken":"syt_ODk0ZGM3ZDUt_njMYMmbScVWzgAzvjKsW_3CivXm"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			atomic.AddInt32(&scrapes, 1)
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"`+testJWT()+`"}}</script>`)
			return
		}
		Expect(r.Header.Get("Authorization")).To(Equal("Bearer "+inner),
			"the bearer must be the envelope's token field, not the envelope")
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	client, err := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookieFromSessionToken(envelope),
		RateLimit: 1000,
	})
	Expect(err).ToNot(HaveOccurred())

	uuid, err := client.AccountUUID(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(uuid).To(Equal(testOwnerUUID))

	_, err = client.ListTags(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(scrapes).To(BeZero(), "the envelope carried a usable JWT — no page fetch should happen")
}

// TestBearerFallsBackToScrape covers the cookies the fast path must reject:
// anything that is not a JWT, and a JWT too close to expiry to use. Both must
// fall through to the __NEXT_DATA__ scrape rather than failing.
func TestBearerFallsBackToScrape(t *testing.T) {
	cases := []struct {
		name   string
		cookie string
	}{
		{"opaque session id", "abc123-not-a-jwt"},
		{"three segments but not base64", "aaa.bbb.ccc"},
		{"jwt already expired", jwtWithExp(1000000000, testOwnerUUID)},
		{"jwt without a uuid claim", jwtWithExp(9999999999, "")},
		{"envelope with an expired token", `{"token":"` + jwtWithExp(1000000000, testOwnerUUID) + `"}`},
		{"envelope with no token field", `{"synapseToken":"syt_abc"}`},
		{"envelope holding something that is not a jwt", `{"token":"abc123"}`},
		// The envelope's inner JWT contains exactly two dots, so splitting the raw
		// envelope on "." yields three parts and a naive parser decodes the middle
		// one as a payload. Rejecting it is the point of this case.
		{"raw envelope must not be mistaken for a jwt", `{"tok":"x.` +
			"eyJleHAiOjk5OTk5OTk5OTksInV1aWQiOiJhYmMifQ" + `.sig"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			var scrapes int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					atomic.AddInt32(&scrapes, 1)
					_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"`+testJWT()+`"}}</script>`)
					return
				}
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + testJWT()))
				_, _ = io.WriteString(w, `[]`)
			}))
			defer srv.Close()

			client, err := NewClient(Options{
				BaseURL:   srv.URL + "/",
				Cookies:   CookieFromSessionToken(tc.cookie),
				RateLimit: 1000,
			})
			Expect(err).ToNot(HaveOccurred())

			_, err = client.ListTags(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(scrapes).To(Equal(int32(1)), "expected a fallback scrape")
		})
	}
}

// TestBearerFromPercentEncodedCookie covers a cookie value that arrived
// URL-escaped (as a raw DevTools header can), which must still be recognised as
// a JWT.
func TestBearerFromPercentEncodedCookie(t *testing.T) {
	RegisterTestingT(t)
	var scrapes int32
	cookieJWT := jwtWithExp(9999999999, testOwnerUUID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			atomic.AddInt32(&scrapes, 1)
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"`+testJWT()+`"}}</script>`)
			return
		}
		Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + cookieJWT))
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	client, err := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookieFromSessionToken(url.QueryEscape(cookieJWT)),
		RateLimit: 1000,
	})
	Expect(err).ToNot(HaveOccurred())

	_, err = client.ListTags(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(scrapes).To(BeZero(), "a percent-encoded JWT cookie should still skip the scrape")
}
