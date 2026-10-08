package familio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	// /biography and source comments this way. A 409 whose body reports the
	// resource missing is ErrNotFound instead.
	ErrConflict = errors.New("familio: version conflict (stale X-Base-Version)")
)

// APIError describes a failed familio.org HTTP response. Every response with a
// status >= 400 is returned as one, so callers can reach the status code and the
// server's message instead of matching on an error string:
//
//	var apiErr *familio.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest { … }
//
// It wraps the sentinel matching its status where one exists (ErrNotFound,
// ErrNotLoggedIn, ErrAccessDenied, ErrConflict), so errors.Is keeps working:
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
// ErrAccessDenied, ErrConflict), or nil when the status maps to none. A 409 is
// ErrConflict unless its Body reports the resource missing, which makes it
// ErrNotFound. It is derived from StatusCode and Body rather than stored, so an
// APIError a caller builds themselves — simulating a 409 in a test, say —
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
	case http.StatusUnauthorized:
		return ErrNotLoggedIn
	case http.StatusForbidden:
		return ErrAccessDenied
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		if reportsNotFound(body) {
			return ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

// codePersonNotFound is the error code GET /persons/<uuid> sends with the 409 it
// answers a missing person with.
const codePersonNotFound = 2604

// reportsNotFound reports whether a 409 body is familio saying the resource does
// not exist rather than that it changed: the person-not-found code, or a message
// containing «не найден» ("not found") — what /persons/<uuid>/sources sends,
// under the generic code 0. A body that is not JSON reports nothing.
func reportsNotFound(body string) bool {
	var msg struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &msg) != nil {
		return false
	}
	return msg.Code == codePersonNotFound || strings.Contains(strings.ToLower(msg.Message), "не найден")
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
