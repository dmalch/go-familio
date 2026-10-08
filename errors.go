package familio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

var (
	// ErrNotFound is returned when a person/event UUID resolves to nothing:
	// HTTP 404, or a 409 whose body says the resource was not found — familio
	// answers a missing person that way on GET /persons/<uuid> and
	// /persons/<uuid>/sources. Resource Read paths use it to drop the resource
	// from state.
	ErrNotFound = errors.New("familio: resource not found")

	// ErrNotLoggedIn is returned when familio.org redirects to a login page, or
	// answers HTTP 401, meaning the session cookie (`t`) is missing or expired
	// and no usable JWT bearer could be obtained from it.
	ErrNotLoggedIn = errors.New("familio: not logged in (session cookie missing or expired)")

	// ErrAccessDenied is returned on HTTP 403: the session is valid but the
	// account may not touch this resource (another owner's person, or a
	// Familio Plus feature on a free account).
	ErrAccessDenied = errors.New("familio: access denied")

	// ErrConflict is returned on HTTP 409 — a stale X-Base-Version
	// optimistic-lock token. The resource changed since it was read; re-read it
	// for a fresh version and retry the write. familio guards /basic,
	// /biography and source comments this way. familio uses 409 for other things
	// too, and those map elsewhere by the body's message: a missing resource is
	// ErrNotFound, a filter that needs a session is ErrNotLoggedIn, and a
	// malformed request is ErrInvalidRequest (see conflictSentinel).
	ErrConflict = errors.New("familio: version conflict (stale X-Base-Version)")

	// ErrInvalidRequest is returned when familio rejects the request itself as
	// malformed: HTTP 400, or a 409 whose message says a parameter is missing or
	// an identifier is invalid. Sending the same request again will not help.
	// The server's message, in APIError.Body, names the problem.
	ErrInvalidRequest = errors.New("familio: invalid request")
)

// APIError describes a failed familio.org HTTP response. Every response with a
// status >= 400 is returned as one, so callers can reach the status code and the
// server's message instead of matching on an error string:
//
//	var apiErr *familio.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest { … }
//
// It wraps the sentinel matching its status where one exists (ErrNotFound,
// ErrNotLoggedIn, ErrAccessDenied, ErrConflict, ErrInvalidRequest), so errors.Is
// keeps working:
//
//	if errors.Is(err, familio.ErrNotFound) { … }
type APIError struct {
	// Method and Path are the request that failed (path only — no host).
	Method string
	Path   string

	// StatusCode is the HTTP status familio.org answered with.
	StatusCode int

	// Body is the response body, truncated to a few hundred bytes. familio's
	// error bodies are JSON like {"type":…,"message":"Ошибка…","code":4}; the
	// API sends every Cyrillic letter as a \uXXXX escape, and Body has those
	// decoded so the message reads as text.
	Body string
}

// Error renders the failed request, its status, what that status means (for the
// four mapped ones), and the server's message. The whole string is what a CLI or
// a Terraform diagnostic shows the user, so it has to stand on its own.
func (e *APIError) Error() string {
	msg := fmt.Sprintf("familio: %s %s: HTTP %d", e.Method, e.Path, e.StatusCode)
	if sentinel := e.Unwrap(); sentinel != nil {
		// The sentinels carry the package prefix; it is already on msg.
		msg += ": " + strings.TrimPrefix(sentinel.Error(), "familio: ")
	}
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

// Unwrap returns the sentinel for this status (ErrNotFound, ErrNotLoggedIn,
// ErrAccessDenied, ErrConflict, ErrInvalidRequest), or nil when the status maps
// to none. A 409 is ErrConflict unless its Body says otherwise (see
// conflictSentinel). It is derived from StatusCode and Body rather than stored,
// so an APIError a caller builds themselves — simulating a 409 in a test, say —
// matches errors.Is exactly like one this package produced.
func (e *APIError) Unwrap() error { return sentinelFor(e.StatusCode, e.Body) }

// newAPIError builds the error for a failed response.
func newAPIError(method, path string, status int, body string) *APIError {
	return &APIError{
		Method:     method,
		Path:       path,
		StatusCode: status,
		Body:       body,
	}
}

// sentinelFor maps a status, and for a 409 its body, to the sentinel callers
// match with errors.Is.
func sentinelFor(status int, body string) error {
	switch status {
	case http.StatusBadRequest:
		return ErrInvalidRequest
	case http.StatusUnauthorized:
		return ErrNotLoggedIn
	case http.StatusForbidden:
		return ErrAccessDenied
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return conflictSentinel(body)
	}
	return nil
}

// codePersonNotFound is the error code GET /persons/<uuid> sends with the 409 it
// answers a missing person with.
const codePersonNotFound = 2604

// invalidRequestMarkers are fragments of the messages familio sends with a 409
// for a malformed request rather than a conflict, all under the generic code 0
// and confirmed live 2026-10-08.
var invalidRequestMarkers = []string{
	"отсутствует параметр",         // «Отсутствует параметр date[till]»: a required parameter is missing
	"недопустимый идентификатор",   // «Недопустимый идентификатор персоны»: a malformed uuid on /events
	"невозможно получить значение", // «Невозможно получить значение»: a malformed uuid on /sources, /biography
}

// conflictSentinel reads a 409's body, since familio answers more than a stale
// X-Base-Version with 409:
//   - a missing resource — the person-not-found code, or a message containing
//     «не найден» ("not found") — is ErrNotFound;
//   - a filter refused without a session («… недоступен без авторизации») is
//     ErrNotLoggedIn;
//   - a malformed request (invalidRequestMarkers) is ErrInvalidRequest;
//   - anything else, including a body that is not JSON, is ErrConflict.
func conflictSentinel(body string) error {
	var msg struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &msg) != nil {
		return ErrConflict
	}
	text := strings.ToLower(msg.Message)
	switch {
	case msg.Code == codePersonNotFound || strings.Contains(text, "не найден"):
		return ErrNotFound
	case strings.Contains(text, "без авторизации"):
		return ErrNotLoggedIn
	case slices.ContainsFunc(invalidRequestMarkers, func(m string) bool { return strings.Contains(text, m) }):
		return ErrInvalidRequest
	}
	return ErrConflict
}

// readableJSON re-encodes a JSON document compactly, in its own key order and
// with its number literals as sent, but with every string written as plain UTF-8
// instead of familio's \uXXXX escapes. Only what JSON requires — quotes,
// backslashes, control characters — stays escaped. Anything that is not valid
// JSON comes back unchanged.
func readableJSON(b []byte) []byte {
	if !json.Valid(b) {
		return b
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)

	// level is an open object or array: whether it holds anything yet, and in
	// an object whether the next token is a value (else a key).
	type level struct{ object, started, afterKey bool }
	var stack []level
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out.Bytes()
		}
		if err != nil {
			return b
		}

		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			stack = stack[:len(stack)-1]
			out.WriteByte(byte(d))
			continue
		}

		isKey := false
		if n := len(stack); n > 0 {
			top := &stack[n-1]
			switch {
			case top.object && !top.afterKey:
				isKey = true
				if top.started {
					out.WriteByte(',')
				}
				top.started, top.afterKey = true, true
			case top.object:
				top.afterKey = false
			default:
				if top.started {
					out.WriteByte(',')
				}
				top.started = true
			}
		}

		switch v := tok.(type) {
		case json.Delim:
			out.WriteByte(byte(v))
			stack = append(stack, level{object: v == '{'})
		case string:
			if enc.Encode(v) != nil {
				return b
			}
			out.Truncate(out.Len() - 1) // Encode's trailing newline
		case json.Number:
			out.WriteString(v.String())
		case bool:
			out.WriteString(strconv.FormatBool(v))
		case nil:
			out.WriteString("null")
		}
		if isKey {
			out.WriteByte(':')
		}
	}
}
