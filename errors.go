package familio

import "errors"

var (
	// ErrNotFound is returned when a person/event UUID resolves to nothing
	// (HTTP 404). Resource Read paths use it to drop the resource from state.
	ErrNotFound = errors.New("familio: resource not found")

	// ErrNotLoggedIn is returned when familio.org redirects to a login page,
	// meaning the session cookie (`t`) is missing or expired.
	ErrNotLoggedIn = errors.New("familio: not logged in (session cookie missing or expired)")

	// ErrAccessDenied is returned on HTTP 401/403.
	ErrAccessDenied = errors.New("familio: access denied")
)
