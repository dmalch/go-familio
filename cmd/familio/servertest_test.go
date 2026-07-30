package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testOwnerUUID is the account uuid carried by the test JWT, so the
// owner-addressed endpoints (tags, matches, history) build the same path the
// commands ask for.
const testOwnerUUID = "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0"

// testJWT builds an unsigned far-future JWT with a uuid claim. The client accepts
// the `t` cookie's own value as its bearer, so handing the CLI this as
// -cookies is enough to authenticate against the fake server.
func testJWT() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"exp":9999999999,"uuid":"` + testOwnerUUID + `"}`))
	return header + "." + payload + ".sig"
}

// serveAPI points the CLI at a fake familio.org for the duration of the test and
// returns the credential flag value to pass with -cookies. handler sees the
// /api/v2/… requests; the token page on / is served for the scrape fallback.
func serveAPI(t *testing.T, handler http.HandlerFunc) (cookies string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"`+testJWT()+`"}}</script>`)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FAMILIO_BASE_URL", srv.URL+"/")
	return "t=" + testJWT()
}

// asMap asserts that a decoded JSON value is an object, so tests can walk a
// request body without unchecked type assertions.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON object, got %T", v)
	}
	return m
}

// asSlice asserts that a decoded JSON value is an array.
func asSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("expected a JSON array, got %T", v)
	}
	return s
}
