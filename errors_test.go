package familio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	. "github.com/onsi/gomega"
)

// Error bodies exactly as familio sent them on 2026-10-08, \u-escaped as on the
// wire, for a person deleted from the account's tree (887aa765-…) and for a
// malformed uuid.
const (
	// GET /persons/<uuid> — a missing person is a 409, not a 404.
	wirePersonNotFound409 = `{"type":"simple_error","message":"Персона не найдена","code":2604}`

	// GET /persons/<uuid>/sources — a 409 with the generic code 0.
	wireSourcesPersonNotFound409 = `{"type":"simple_error","message":"Не найдена персона 887aa765-fda7-4644-99cc-6b6d8f103167","code":0}`

	// GET /persons/not-a-uuid/sources — also a 409, but not a missing person.
	wireMalformedUUID409 = `{"type":"simple_error","message":"Невозможно получить значение","code":0}`
)

// TestMissingPersonIsNotFound covers the reads familio answers with a 409 for a
// person that does not exist: they must report ErrNotFound — what a caller
// drops a deleted resource on — and not ErrConflict, which means "re-read and
// retry".
func TestMissingPersonIsNotFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		call func(*Client) error
	}{
		{"GetPersonRegular", wirePersonNotFound409, func(c *Client) error {
			_, err := c.GetPersonRegular(context.Background(), "gone")
			return err
		}},
		{"GetPersonDisplay", wirePersonNotFound409, func(c *Client) error {
			_, err := c.GetPersonDisplay(context.Background(), "gone")
			return err
		}},
		{"GetPersonSources", wireSourcesPersonNotFound409, func(c *Client) error {
			_, err := c.GetPersonSources(context.Background(), "gone")
			return err
		}},
		{"UpdateSourceComment", wireSourcesPersonNotFound409, func(c *Client) error {
			_, err := c.UpdateSourceComment(context.Background(), "gone", "s-1", "x")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, tc.body)
			})
			defer srv.Close()

			err := tc.call(newTestClient(srv))

			Expect(err).To(MatchError(ErrNotFound))
			Expect(errors.Is(err, ErrConflict)).To(BeFalse(), "a missing person is not a version conflict")
			var apiErr *APIError
			Expect(errors.As(err, &apiErr)).To(BeTrue())
			Expect(apiErr.StatusCode).To(Equal(http.StatusConflict), "the wire status is kept")
			Expect(err.Error()).To(ContainSubstring("resource not found"))
			Expect(err.Error()).To(ContainSubstring("найдена"), "the message reads as text, not \\u escapes")
		})
	}
}

// TestOtherConflictsStayConflicts checks the not-found reading is narrow: a 409
// that does not say the resource is missing is still ErrConflict.
func TestOtherConflictsStayConflicts(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"no body", ``},
		{"not JSON", `<html>conflict</html>`},
		{"sole birth event", `{"message":"Нельзя удалить единственное событие рождения"}`},
		{"malformed uuid", wireMalformedUUID409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := error(newAPIError(http.MethodGet, "/api/v2/x", http.StatusConflict, snippet([]byte(tc.body))))

			Expect(err).To(MatchError(ErrConflict))
			Expect(errors.Is(err, ErrNotFound)).To(BeFalse())
		})
	}
}

// TestAPIErrorByHandReadsTheBody keeps the 0.7.1 property: Unwrap is derived
// from the exported fields, so a caller simulating familio's missing-person 409
// gets the same answer this package would.
func TestAPIErrorByHandReadsTheBody(t *testing.T) {
	RegisterTestingT(t)

	Expect(&APIError{StatusCode: http.StatusConflict, Body: `{"message":"Персона не найдена","code":2604}`}).
		To(MatchError(ErrNotFound))
	Expect(&APIError{StatusCode: http.StatusConflict, Body: wirePersonNotFound409}).
		To(MatchError(ErrNotFound), "an escaped body is read too")
	Expect(&APIError{StatusCode: http.StatusConflict}).To(MatchError(ErrConflict))
}

// TestAPIErrorBodyIsReadable checks the body an error shows has familio's
// \uXXXX escapes decoded — otherwise every Cyrillic message reaches a CLI or a
// Terraform diagnostic as escape soup — with the server's key order kept.
func TestAPIErrorBodyIsReadable(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"type":"simple_error","message":"Персона 887aa765 не найдена","code":3}`)
	})
	defer srv.Close()

	_, err := newTestClient(srv).GetPersonBasic(context.Background(), "887aa765")

	var apiErr *APIError
	Expect(errors.As(err, &apiErr)).To(BeTrue())
	Expect(apiErr.Body).To(Equal(`{"type":"simple_error","message":"Персона 887aa765 не найдена","code":3}`))
	Expect(err.Error()).To(HaveSuffix(`: {"type":"simple_error","message":"Персона 887aa765 не найдена","code":3}`))
}

// TestSnippetTruncatesOnARuneBoundary checks a long body is cut without
// splitting a multi-byte letter, which would leave invalid UTF-8 in the message.
func TestSnippetTruncatesOnARuneBoundary(t *testing.T) {
	RegisterTestingT(t)
	for _, prefix := range []string{"", "x", "xy"} {
		got := snippet([]byte(prefix + strings.Repeat("Ж", 400)))

		Expect(utf8.ValidString(got)).To(BeTrue(), "prefix %q", prefix)
		Expect(got).To(HaveSuffix("Ж…"))
		Expect(len(got)).To(BeNumerically("<=", 300+len("…")))
	}
}

// TestReadableJSON pins the re-encoding: escapes decoded, everything else — key
// order, number literals, nesting, the escapes JSON cannot do without — kept.
func TestReadableJSON(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"b":"Ж","a":[1.50,-2e3,true,null,{}],"c":{"d":[]}}`, `{"b":"Ж","a":[1.50,-2e3,true,null,{}],"c":{"d":[]}}`},
		{` [ "a\"b\\c\n\u0007" , "<>&" ] `, `["a\"b\\c\n\u0007","<>&"]`},
		{`"П"`, `"П"`},
		{`42`, `42`},
		{`not json`, `not json`},
		{``, ``},
	} {
		t.Run(tc.in, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(string(readableJSON([]byte(tc.in)))).To(Equal(tc.want))
		})
	}
}
