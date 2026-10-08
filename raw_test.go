package familio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	. "github.com/onsi/gomega"
)

// TestDoRawSendsTheTypedCallsHeaders locks what DoRaw adds to a request: the
// bearer, the User-Agent, and the ld+json negotiation the typed calls use, with
// the body sent as given.
func TestDoRawSendsTheTypedCallsHeaders(t *testing.T) {
	RegisterTestingT(t)
	var got *http.Request
	var gotBody string
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":7}`)
	})
	defer srv.Close()

	resp, err := newTestClient(srv).DoRaw(context.Background(), http.MethodPost, "tags", nil,
		[]byte(`{"tag":"Метка"}`))

	Expect(err).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(string(resp.Body)).To(Equal(`{"id":7}`))
	Expect(got.Method).To(Equal(http.MethodPost))
	Expect(got.URL.Path).To(Equal("/api/v2/tags"))
	Expect(got.Header.Get("Authorization")).To(Equal("Bearer " + testJWT()))
	Expect(got.Header.Get("User-Agent")).To(Equal(defaultUserAgent))
	Expect(got.Header.Get("Accept")).To(Equal("application/ld+json"))
	Expect(got.Header.Get("Content-Type")).To(Equal("application/ld+json"))
	Expect(gotBody).To(Equal(`{"tag":"Метка"}`))
}

// TestDoRawEndpointForms checks every way of naming an endpoint lands on the
// same /api/v2 path, query included — so familio's own "/api/v2/…" links and
// full URLs can be passed back in.
func TestDoRawEndpointForms(t *testing.T) {
	var gotURI atomic.Value
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotURI.Store(r.URL.RequestURI())
		_, _ = io.WriteString(w, `{}`)
	})
	defer srv.Close()

	for _, endpoint := range []string{
		"persons/p-1/events?x=1",
		"/persons/p-1/events?x=1",
		"api/v2/persons/p-1/events?x=1",
		"/api/v2/persons/p-1/events?x=1",
		srv.URL + "/api/v2/persons/p-1/events?x=1",
	} {
		t.Run(endpoint, func(t *testing.T) {
			RegisterTestingT(t)
			_, err := newTestClient(srv).DoRaw(context.Background(), http.MethodGet, endpoint, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(gotURI.Load()).To(Equal("/api/v2/persons/p-1/events?x=1"))
		})
	}
}

// TestDoRawRefusesForeignURLs is the guard that keeps the bearer on familio.org:
// a URL anywhere but under the client's API root, or no endpoint at all, fails
// before any request is made.
func TestDoRawRefusesForeignURLs(t *testing.T) {
	var hits atomic.Int32
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, `{}`)
	})
	defer srv.Close()

	for _, tc := range []struct{ endpoint, want string }{
		{"https://evil.example/api/v2/profile", "not a familio.org API URL"},
		{"http://evil.example/api/v2/profile", "not a familio.org API URL"},
		{srv.URL + "/profile", "not a familio.org API URL"},
		{"", "no endpoint"},
		{"/", "no endpoint"},
		{"/api/v2/", "no endpoint"},
		{"?page=2", "no endpoint"},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			RegisterTestingT(t)
			_, err := newTestClient(srv).DoRaw(context.Background(), http.MethodGet, tc.endpoint, nil, nil)
			Expect(err).To(MatchError(ContainSubstring(tc.want)))
		})
	}
	Expect(hits.Load()).To(BeZero())
}

// TestDoRawReturnsErrorStatuses checks a status >= 400 is the caller's to show:
// it comes back as a response with the server's whole body, not as an
// *APIError with a truncated snippet.
func TestDoRawReturnsErrorStatuses(t *testing.T) {
	RegisterTestingT(t)
	long := `{"message":"` + strings.Repeat("Ошибка ", 100) + `"}`
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Debug", "yes")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, long)
	})
	defer srv.Close()

	resp, err := newTestClient(srv).DoRaw(context.Background(), http.MethodGet, "persons/nope", nil, nil)

	Expect(err).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	Expect(resp.Header.Get("X-Debug")).To(Equal("yes"))
	Expect(string(resp.Body)).To(Equal(long))
}

// TestDoRawRetriesWithTheBody checks a retried request carries its body again:
// the first attempt drains it, so a retry that did not rewind would go out empty.
func TestDoRawRetriesWithTheBody(t *testing.T) {
	RegisterTestingT(t)
	var bodies []string
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	})
	defer srv.Close()

	resp, err := newTestClient(srv).DoRaw(context.Background(), http.MethodPost, "tags/get-by-persons-id-list", nil,
		[]byte(`["p-1"]`))

	Expect(err).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(bodies).To(Equal([]string{`["p-1"]`, `["p-1"]`}))
}

// TestDoRawWithoutSessionIsAnonymous covers the public endpoint: with no `t`
// cookie DoRaw sends no bearer and does not try to scrape one.
func TestDoRawWithoutSessionIsAnonymous(t *testing.T) {
	RegisterTestingT(t)
	var scraped atomic.Bool
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			scraped.Store(true)
			return
		}
		auth.Store(r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"pager":{"totalItems":0},"data":[]}`)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookiesFromHeader("cookieConfirmed=1"),
		RateLimit: 1000,
	})
	resp, err := client.DoRaw(context.Background(), http.MethodGet, "persons?settlement=s-1", nil, nil)

	Expect(err).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(auth.Load()).To(Equal(""))
	Expect(scraped.Load()).To(BeFalse())
}

// TestDoRawHeaderOverridesDefaults checks the caller's headers are applied last.
func TestDoRawHeaderOverridesDefaults(t *testing.T) {
	RegisterTestingT(t)
	var got http.Header
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header
		_, _ = io.WriteString(w, `{}`)
	})
	defer srv.Close()

	header := http.Header{}
	header.Set("Accept", "application/json")
	header.Set("X-Base-Version", "v-1")
	_, err := newTestClient(srv).DoRaw(context.Background(), http.MethodGet, "profile", header, nil)

	Expect(err).ToNot(HaveOccurred())
	Expect(got.Get("Accept")).To(Equal("application/json"))
	Expect(got.Get("X-Base-Version")).To(Equal("v-1"))
	Expect(got.Get("Content-Type")).To(BeEmpty())
}
