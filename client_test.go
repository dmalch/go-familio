package familio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

// TestDefaultUserAgentIdentifiesThisLibrary pins the identity the client presents
// to familio.org: this module and its version, not whatever consumer it was
// extracted from.
func TestDefaultUserAgentIdentifiesThisLibrary(t *testing.T) {
	RegisterTestingT(t)
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = io.WriteString(w, `{"pager":{"totalItems":0},"data":[]}`)
	}))
	defer srv.Close()

	client, err := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	Expect(err).ToNot(HaveOccurred())
	_, err = client.ListSettlementPersons(context.Background(), "settlement-uuid")
	Expect(err).ToNot(HaveOccurred())

	Expect(got).To(Equal("go-familio/" + Version + " (+https://github.com/dmalch/go-familio)"))
	Expect(Version).ToNot(BeEmpty())
}

// TestUserAgentOverride keeps the Options escape hatch working.
func TestUserAgentOverride(t *testing.T) {
	RegisterTestingT(t)
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = io.WriteString(w, `{"pager":{"totalItems":0},"data":[]}`)
	}))
	defer srv.Close()

	client, err := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000, UserAgent: "my-tool/2.0"})
	Expect(err).ToNot(HaveOccurred())
	_, err = client.ListSettlementPersons(context.Background(), "settlement-uuid")
	Expect(err).ToNot(HaveOccurred())

	Expect(got).To(Equal("my-tool/2.0"))
}

// TestRedirectToLoginIsNotLoggedIn locks the CheckRedirect guard: familio answers
// an unauthenticated request by redirecting to its login page, and following that
// would decode an HTML form as JSON. The client must surface ErrNotLoggedIn
// instead — and, since there is no HTTP response to describe, as the bare
// sentinel rather than an *APIError.
func TestRedirectToLoginIsNotLoggedIn(t *testing.T) {
	for _, path := range []string{"/login", "/auth/signin", "/ru/signin"} {
		t.Run(path, func(t *testing.T) {
			RegisterTestingT(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == path {
					_, _ = io.WriteString(w, "<html>login form</html>")
					return
				}
				http.Redirect(w, r, path, http.StatusFound)
			}))
			defer srv.Close()

			client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
			_, err := client.ListSettlementPersons(context.Background(), "whatever")
			Expect(err).To(MatchError(ErrNotLoggedIn))

			var apiErr *APIError
			Expect(errors.As(err, &apiErr)).To(BeFalse(), "a login bounce is not an HTTP status")
		})
	}
}

// TestRedirectElsewhereIsFollowed keeps ordinary redirects working — only login
// paths are treated as a dead session.
func TestRedirectElsewhereIsFollowed(t *testing.T) {
	RegisterTestingT(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/elsewhere" {
			_, _ = io.WriteString(w, `{"pager":{"totalItems":0},"data":[]}`)
			return
		}
		http.Redirect(w, r, "/api/v2/elsewhere", http.StatusFound)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	_, err := client.ListSettlementPersons(context.Background(), "whatever")
	Expect(err).ToNot(HaveOccurred())
}
