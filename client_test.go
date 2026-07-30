package familio

import (
	"context"
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
