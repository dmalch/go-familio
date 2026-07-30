package familio

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	// ErrNotFound is returned when a person/event UUID resolves to nothing
	// (HTTP 404). Resource Read paths use it to drop the resource from state.
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
	// /biography and source comments this way.
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
	// error bodies are JSON like {"type":…,"message":"Ошибка…","code":4}.
	Body string

	// err is the wrapped sentinel, if the status maps to one.
	err error
}

// Error renders the failed request, its status, and the server's message.
func (e *APIError) Error() string {
	msg := fmt.Sprintf("familio: %s %s: HTTP %d", e.Method, e.Path, e.StatusCode)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

// Unwrap returns the sentinel for this status (ErrNotFound, ErrNotLoggedIn,
// ErrAccessDenied, ErrConflict), or nil when the status maps to none.
func (e *APIError) Unwrap() error { return e.err }

// newAPIError builds the error for a failed response, attaching the sentinel
// that matches its status.
func newAPIError(method, path string, status int, body string) *APIError {
	return &APIError{
		Method:     method,
		Path:       path,
		StatusCode: status,
		Body:       body,
		err:        sentinelFor(status),
	}
}

// sentinelFor maps a status to the sentinel callers match with errors.Is.
func sentinelFor(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return ErrNotLoggedIn
	case http.StatusForbidden:
		return ErrAccessDenied
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	}
	return nil
}
