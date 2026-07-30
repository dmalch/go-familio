package familio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	. "github.com/onsi/gomega"
)

// TestAPIErrorCarriesStatus locks the structured error: a plain 400 (no sentinel
// of its own) still reaches the caller as an *APIError carrying the status, the
// request that failed, and the server's message.
func TestAPIErrorCarriesStatus(t *testing.T) {
	RegisterTestingT(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"Ошибка(и) в данных запроса"}`)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	_, err := client.ListSettlementPersons(context.Background(), "whatever")

	var apiErr *APIError
	Expect(errors.As(err, &apiErr)).To(BeTrue(), "expected an *APIError, got %v", err)
	Expect(apiErr.StatusCode).To(Equal(http.StatusBadRequest))
	Expect(apiErr.Method).To(Equal(http.MethodGet))
	Expect(apiErr.Path).To(Equal("/api/v2/persons"))
	Expect(apiErr.Body).To(ContainSubstring("Ошибка(и) в данных запроса"))
	Expect(apiErr.Error()).To(ContainSubstring("400"))
}

// TestAPIErrorWrapsSentinels pins the compatibility promise: the sentinels stay
// reachable with errors.Is through the new *APIError, so existing callers that
// only check errors.Is(err, ErrNotFound) keep working.
func TestAPIErrorWrapsSentinels(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"404 is not found", http.StatusNotFound, ErrNotFound},
		{"401 is not logged in", http.StatusUnauthorized, ErrNotLoggedIn},
		{"403 is access denied", http.StatusForbidden, ErrAccessDenied},
		{"409 is a version conflict", http.StatusConflict, ErrConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
			_, err := client.ListSettlementPersons(context.Background(), "whatever")

			Expect(err).To(MatchError(tc.want))

			var apiErr *APIError
			Expect(errors.As(err, &apiErr)).To(BeTrue(), "expected an *APIError, got %v", err)
			Expect(apiErr.StatusCode).To(Equal(tc.status))
		})
	}
}

// TestAPIErrorMessageExplainsTheStatus keeps the error text self-explanatory: a
// bare 401 must still say what it means, because that string is what a CLI or a
// Terraform diagnostic shows the user.
func TestAPIErrorMessageExplainsTheStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   []string
	}{
		{"401 names the session problem", http.StatusUnauthorized, "",
			[]string{"HTTP 401", "not logged in"}},
		{"409 names the version conflict", http.StatusConflict, "",
			[]string{"HTTP 409", "stale X-Base-Version"}},
		{"the server message is kept too", http.StatusUnauthorized, `{"message":"Требуется авторизация"}`,
			[]string{"HTTP 401", "not logged in", "Требуется авторизация"}},
		{"an unmapped status just reports itself", http.StatusBadRequest, `{"message":"Ошибка"}`,
			[]string{"HTTP 400", "Ошибка"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
			_, err := client.ListSettlementPersons(context.Background(), "whatever")
			for _, want := range tc.want {
				Expect(err.Error()).To(ContainSubstring(want))
			}
			// The sentinel's own "familio: " prefix must not be repeated inline.
			Expect(err.Error()).To(HavePrefix("familio: "))
			Expect(strings.Count(err.Error(), "familio: ")).To(Equal(1))
		})
	}
}

// TestDoRetriesAndReplaysBody covers the retry path for a request that carries a
// body: the 429 must be retried and the POST body re-sent, not swallowed by the
// first attempt's consumed reader.
func TestDoRetriesAndReplaysBody(t *testing.T) {
	RegisterTestingT(t)
	var attempts int32
	var bodies []string

	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `[{"id":1,"tag":"Метка","color":"mint-mist","description":"","isFree":true}]`)
	})
	defer srv.Close()

	tags, err := newTestClient(srv).AssignPersonTags(context.Background(), "person-uuid", []int{1})
	Expect(err).ToNot(HaveOccurred())
	Expect(tags).To(HaveLen(1))
	Expect(attempts).To(Equal(int32(2)), "the 429 should have been retried")
	Expect(bodies).To(HaveLen(2))
	Expect(bodies[1]).To(Equal(bodies[0]), "the retry must re-send the same body")
	Expect(bodies[1]).To(Equal("[1]"))
}

// TestDoReturnsAPIErrorAfterRetriesExhausted checks the give-up path still hands
// back the structured error rather than a bare string.
func TestDoReturnsAPIErrorAfterRetriesExhausted(t *testing.T) {
	RegisterTestingT(t)
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	_, err := client.ListSettlementPersons(context.Background(), "whatever")

	var apiErr *APIError
	Expect(errors.As(err, &apiErr)).To(BeTrue(), "expected an *APIError, got %v", err)
	Expect(apiErr.StatusCode).To(Equal(http.StatusBadGateway))
	Expect(attempts).To(BeNumerically(">", 1), "5xx should have been retried")
}
