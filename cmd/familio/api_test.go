package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
)

// seenRequest is what the fake familio.org saw of one /api/v2 request.
type seenRequest struct {
	method string
	path   string
	query  url.Values
	header http.Header
	body   string
}

// recordAPI is serveAPI that also records every /api/v2 request, answering each
// with respond. It returns the -cookies value and a snapshot of what was seen.
func recordAPI(t *testing.T, respond http.HandlerFunc) (cookies string, seen func() []seenRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []seenRequest
	cookies = serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, seenRequest{
			method: r.Method, path: r.URL.Path, query: r.URL.Query(), header: r.Header.Clone(), body: string(body),
		})
		mu.Unlock()
		respond(w, r)
	})
	return cookies, func() []seenRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenRequest(nil), requests...)
	}
}

// runAPICLI runs the CLI isolated from the developer's own credentials, so a
// test only authenticates when it passes -cookies itself.
func runAPICLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("FAMILIO_COOKIES", "")
	t.Setenv("FAMILIO_SESSION", "")
	t.Setenv("FAMILIO_BROWSER", "")
	return runStdin(stdin, args...)
}

func respondJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/ld+json")
		_, _ = io.WriteString(w, body)
	}
}

// TestRunAPI_GetPutsFieldsInTheQuery covers the common read: the endpoint's
// {owner} becomes the account uuid, -f fields join the query with their PHP
// bracket keys intact, the bearer is sent, and the JSON answer is re-indented.
func TestRunAPI_GetPutsFieldsInTheQuery(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, respondJSON(`{"data":[{"record":{"id":1,"operation":"update"}}]}`))

	code, out, errb := runAPICLI(t, "", "-cookies", cookies, "api", "-X", "GET", "persons/history/{owner}?orderBy=id",
		"-f", "page=1", "-f", "operation[]=update", "-f", "operation[]=create", "-f", "text=Тюжин")

	g.Expect(code).To(Equal(0), errb)
	reqs := seen()
	g.Expect(reqs).To(HaveLen(1))
	g.Expect(reqs[0].method).To(Equal(http.MethodGet))
	g.Expect(reqs[0].path).To(Equal("/api/v2/persons/history/" + testOwnerUUID))
	g.Expect(reqs[0].query).To(Equal(url.Values{
		"orderBy": {"id"}, "page": {"1"}, "operation[]": {"update", "create"}, "text": {"Тюжин"},
	}))
	g.Expect(reqs[0].header.Get("Authorization")).To(Equal("Bearer " + testJWT()))
	g.Expect(reqs[0].header.Get("Accept")).To(Equal("application/ld+json"))
	g.Expect(reqs[0].body).To(BeEmpty())
	g.Expect(out).To(Equal("{\n  \"data\": [\n    {\n      \"record\": {\n        \"id\": 1,\n" +
		"        \"operation\": \"update\"\n      }\n    }\n  ]\n}\n"))
}

// TestRunAPI_FieldsMakeAJSONPost checks fields default the method to POST and
// form a typed, nested ld+json object, with Cyrillic sent as plain UTF-8.
func TestRunAPI_FieldsMakeAJSONPost(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":42,"tag":"Проверить"}`)
	})

	code, out, errb := runAPICLI(t, "", "-cookies", cookies, "api", "tags",
		"-f", "tag=Проверить", "-F", "color=mint-mist", "-F", "meta[pinned]=true", "-F", "meta[rank]=3",
		"-F", "owners[]={owner}", "-f", "raw={owner}")

	g.Expect(code).To(Equal(0), errb)
	reqs := seen()
	g.Expect(reqs).To(HaveLen(1))
	g.Expect(reqs[0].method).To(Equal(http.MethodPost))
	g.Expect(reqs[0].path).To(Equal("/api/v2/tags"))
	g.Expect(reqs[0].query).To(BeEmpty())
	g.Expect(reqs[0].header.Get("Content-Type")).To(Equal("application/ld+json"))
	g.Expect(reqs[0].body).To(ContainSubstring(`"tag":"Проверить"`), "UTF-8, not \\u escapes")

	var body any
	g.Expect(json.Unmarshal([]byte(reqs[0].body), &body)).To(Succeed())
	g.Expect(body).To(Equal(map[string]any{
		"tag":    "Проверить",
		"color":  "mint-mist",
		"meta":   map[string]any{"pinned": true, "rank": float64(3)},
		"owners": []any{testOwnerUUID},
		"raw":    "{owner}",
	}), "-F fills {owner}, -f leaves it alone")
	g.Expect(out).To(ContainSubstring(`"tag": "Проверить"`))
}

// TestRunAPI_InputSendsTheBodyAsIs covers the bare-array bodies familio's
// *-by-ids endpoints take: -input - sends stdin untouched, and any fields move
// to the query.
func TestRunAPI_InputSendsTheBodyAsIs(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, respondJSON(`[]`))

	code, _, errb := runAPICLI(t, `["m-1","m-2"]`, "-cookies", cookies, "api",
		"users/{owner}/matches/confirm-by-ids", "-input", "-", "-f", "dry=1")

	g.Expect(code).To(Equal(0), errb)
	reqs := seen()
	g.Expect(reqs).To(HaveLen(1))
	g.Expect(reqs[0].method).To(Equal(http.MethodPost))
	g.Expect(reqs[0].path).To(Equal("/api/v2/users/" + testOwnerUUID + "/matches/confirm-by-ids"))
	g.Expect(reqs[0].query).To(Equal(url.Values{"dry": {"1"}}))
	g.Expect(reqs[0].body).To(Equal(`["m-1","m-2"]`))
}

// TestRunAPI_MethodAndHeaders checks -X and -H, which override the defaults —
// how a caller sends familio's X-Base-Version optimistic lock.
func TestRunAPI_MethodAndHeaders(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, respondJSON(`{}`))

	code, _, errb := runAPICLI(t, "", "-cookies", cookies, "api", "-X", "put", "persons/p-1/biography",
		"-H", "X-Base-Version: 2026-07-27T08:23:44+00:00", "-H", "Accept: application/json", "-f", "text=Био")

	g.Expect(code).To(Equal(0), errb)
	reqs := seen()
	g.Expect(reqs).To(HaveLen(1))
	g.Expect(reqs[0].method).To(Equal(http.MethodPut))
	g.Expect(reqs[0].header.Get("X-Base-Version")).To(Equal("2026-07-27T08:23:44+00:00"))
	g.Expect(reqs[0].header.Get("Accept")).To(Equal("application/json"))
	g.Expect(reqs[0].body).To(Equal(`{"text":"Био"}`))
}

// TestRunAPI_IncludePrintsStatusAndHeaders covers -i, given after the endpoint.
func TestRunAPI_IncludePrintsStatusAndHeaders(t *testing.T) {
	g := NewWithT(t)
	cookies, _ := recordAPI(t, respondJSON(`{"user":{"uuid":"u"}}`))

	code, out, errb := runAPICLI(t, "", "api", "profile", "-i", "-cookies", cookies)

	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(HavePrefix("HTTP 200 OK\n"))
	g.Expect(out).To(ContainSubstring("Content-Type: application/ld+json\n"))
	g.Expect(out).To(ContainSubstring("\n\n{\n  \"user\""))
}

// TestRunAPI_ErrorStatusPrintsBodyAndFails checks a failed call shows the
// server's whole message on stdout and exits 1 with the status on stderr.
func TestRunAPI_ErrorStatusPrintsBodyAndFails(t *testing.T) {
	g := NewWithT(t)
	cookies, _ := recordAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":404,"message":"Персона не найдена"}`)
	})

	code, out, errb := runAPICLI(t, "", "-cookies", cookies, "api", "persons/nope")

	g.Expect(code).To(Equal(1))
	g.Expect(out).To(ContainSubstring(`"message": "Персона не найдена"`))
	g.Expect(errb).To(Equal("familio api: HTTP 404 Not Found\n"))
}

// TestRunAPI_NoContentPrintsNothing covers familio's 204 on deletes.
func TestRunAPI_NoContentPrintsNothing(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, out, errb := runAPICLI(t, "", "-cookies", cookies, "api", "-X", "DELETE", "tags/7")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(BeEmpty())
	g.Expect(seen()[0].method).To(Equal(http.MethodDelete))
}

// TestRunAPI_NonJSONIsPrintedAsIs checks a body that is not JSON is not mangled.
func TestRunAPI_NonJSONIsPrintedAsIs(t *testing.T) {
	g := NewWithT(t)
	cookies, _ := recordAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "plain text")
	})

	code, out, errb := runAPICLI(t, "", "-cookies", cookies, "api", "coral/path")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(Equal("plain text"))
}

// TestRunAPI_PublicCallNeedsNoCredentials covers the public settlement list:
// with no credentials the call goes out anonymously instead of failing.
func TestRunAPI_PublicCallNeedsNoCredentials(t *testing.T) {
	g := NewWithT(t)
	_, seen := recordAPI(t, respondJSON(`{"pager":{"totalItems":0},"data":[]}`))

	code, _, errb := runAPICLI(t, "", "api", "persons?settlement=s-1")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(seen()[0].header.Get("Authorization")).To(BeEmpty())
}

// TestRunAPI_OwnerNeedsASession checks {owner} is a credential check of its own:
// with no session the page renders logged out, and no API call is made.
func TestRunAPI_OwnerNeedsASession(t *testing.T) {
	g := NewWithT(t)
	var apiCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{}}</script>`)
			return
		}
		apiCalls++
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FAMILIO_BASE_URL", srv.URL+"/")

	code, _, errb := runAPICLI(t, "", "api", "users/{owner}/tags")

	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("resolve {owner}"))
	g.Expect(errb).To(ContainSubstring("not logged in"))
	g.Expect(apiCalls).To(BeZero())
}

// TestRunAPI_RefusesAForeignURL keeps the bearer on familio.org.
func TestRunAPI_RefusesAForeignURL(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, respondJSON(`{}`))

	code, _, errb := runAPICLI(t, "", "-cookies", cookies, "api", "https://evil.example/api/v2/profile")

	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("not a familio.org API URL"))
	g.Expect(seen()).To(BeEmpty())
}

// TestRunAPI_PaginateFollowsPages covers the {page, itemsPerPage, totalItems}
// pager: it asks for the next page, keeping the page size, until the total is
// reached, and prints every page.
func TestRunAPI_PaginateFollowsPages(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			_, _ = io.WriteString(w, `{"data":[{"id":1},{"id":2}],"pager":{"page":1,"itemsPerPage":2,"totalItems":3}}`)
		case "2":
			_, _ = io.WriteString(w, `{"data":[{"id":3}],"pager":{"page":2,"itemsPerPage":2,"totalItems":3}}`)
		default:
			t.Errorf("paged past the end: %s", r.URL.RawQuery)
		}
	})

	code, out, errb := runAPICLI(t, "", "-cookies", cookies, "api", "-paginate", "-X", "GET",
		"persons/history/{owner}", "-f", "itemsPerPage=2", "-f", "orderBy=id")

	g.Expect(code).To(Equal(0), errb)
	reqs := seen()
	g.Expect(reqs).To(HaveLen(2))
	g.Expect(reqs[0].query).To(Equal(url.Values{"itemsPerPage": {"2"}, "orderBy": {"id"}}))
	g.Expect(reqs[1].query).To(Equal(url.Values{"itemsPerPage": {"2"}, "orderBy": {"id"}, "page": {"2"}}))
	g.Expect(out).To(ContainSubstring(`"id": 1`))
	g.Expect(out).To(ContainSubstring(`"id": 3`))
}

// TestRunAPI_PaginateFollowsTheCursor covers the {lastItem, hasMore} pager on a
// POST read: the cursor moves to the query and the filter body is resent.
func TestRunAPI_PaginateFollowsTheCursor(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("pageAfterItem") {
		case "":
			_, _ = io.WriteString(w, `{"data":[{"uuid":"m1"}],"pager":{"lastItem":"m1","hasMore":true}}`)
		case "m1":
			_, _ = io.WriteString(w, `{"data":[{"uuid":"m2"}],"pager":{"lastItem":"m2","hasMore":false}}`)
		default:
			t.Errorf("scrolled past the end: %s", r.URL.RawQuery)
		}
	})

	code, out, errb := runAPICLI(t, `{"status":["undecided"]}`, "-cookies", cookies, "api", "-paginate",
		"users/{owner}/matches/get-by-filters-scroll?itemsPerPage=1", "-input", "-")

	g.Expect(code).To(Equal(0), errb)
	reqs := seen()
	g.Expect(reqs).To(HaveLen(2))
	for _, r := range reqs {
		g.Expect(r.method).To(Equal(http.MethodPost))
		g.Expect(r.body).To(Equal(`{"status":["undecided"]}`))
		g.Expect(r.query.Get("itemsPerPage")).To(Equal("1"))
	}
	g.Expect(reqs[1].query.Get("pageAfterItem")).To(Equal("m1"))
	g.Expect(out).To(ContainSubstring(`"uuid": "m1"`))
	g.Expect(out).To(ContainSubstring(`"uuid": "m2"`))
}

// TestRunAPI_PaginateStopsWhenThePageDoesNotAdvance guards against a server
// that ignores the page parameter, which would otherwise page forever.
func TestRunAPI_PaginateStopsWhenThePageDoesNotAdvance(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, respondJSON(
		`{"data":[{"id":1}],"pager":{"page":1,"itemsPerPage":1,"totalItems":5}}`))

	code, _, errb := runAPICLI(t, "", "-cookies", cookies, "api", "-paginate", "persons/history/x")

	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("page=2"))
	g.Expect(seen()).To(HaveLen(2))
}

// TestRunAPI_PaginateWithoutAPagerIsOnePage checks -paginate on a response with
// no pager envelope just prints it.
func TestRunAPI_PaginateWithoutAPagerIsOnePage(t *testing.T) {
	g := NewWithT(t)
	cookies, seen := recordAPI(t, respondJSON(`[{"id":1}]`))

	code, _, errb := runAPICLI(t, "", "-cookies", cookies, "api", "-paginate", "users/u/tags")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(seen()).To(HaveLen(1))
}

// TestRunAPI_Usage covers the argument checks, made before any request.
func TestRunAPI_Usage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no endpoint", []string{"api"}, "usage: familio api"},
		{"two endpoints", []string{"api", "profile", "tree"}, "usage: familio api"},
		{"paginate a PUT", []string{"api", "-paginate", "-X", "PUT", "tags/1"}, "-paginate"},
		{"bad header", []string{"api", "-H", "nocolon", "profile"}, "Name: value"},
		{"bad field", []string{"api", "-f", "novalue", "profile"}, "key=value"},
		{"stdin twice", []string{"api", "-input", "-", "-F", "a=@-", "tags"}, "stdin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			_, seen := recordAPI(t, respondJSON(`{}`))

			code, _, errb := runAPICLI(t, "", tc.args...)

			g.Expect(code).To(Equal(1))
			g.Expect(errb).To(ContainSubstring(tc.want))
			g.Expect(seen()).To(BeEmpty())
		})
	}
}

// TestRun_Help_ListsAPI keeps the command discoverable.
func TestRun_Help_ListsAPI(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	g.Expect(out).To(MatchRegexp(`(?m)^  api\s+call any`))
}

// TestIndentJSON pins the printer: familio's API escapes every Cyrillic letter
// as \uXXXX, which the output must undo, while keeping the server's key order,
// its number literals, and the escapes JSON cannot do without.
func TestIndentJSON(t *testing.T) {
	g := NewWithT(t)
	var out bytes.Buffer

	err := indentJSON(&out, []byte(
		`{"displayName":"Мальчиков Д.","zeta":1.50,"alpha":[],`+
			`"obj":{},"esc":"a\"b\\c\nd<>&\u0007","list":[true,null,{"k":-2e3}]}`))

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(out.String()).To(Equal(`{
  "displayName": "Мальчиков Д.",
  "zeta": 1.50,
  "alpha": [],
  "obj": {},
  "esc": "a\"b\\c\nd<>&\u0007",
  "list": [
    true,
    null,
    {
      "k": -2e3
    }
  ]
}`))
}

// TestIndentJSON_MatchesTheStandardIndent checks the printer agrees with
// json.Indent wherever escapes are not involved.
func TestIndentJSON_MatchesTheStandardIndent(t *testing.T) {
	for _, doc := range []string{
		`[]`, `{}`, `"x"`, `42`, `null`,
		`[[],[{}],[1,[2,[3]]]]`,
		`{"a":{"b":{"c":[1,2,{"d":"e"}]}},"f":[{"g":[]}]}`,
		` { "spaced" : [ 1 , 2 ] } `,
	} {
		t.Run(doc, func(t *testing.T) {
			g := NewWithT(t)
			var got, want bytes.Buffer
			g.Expect(indentJSON(&got, []byte(doc))).To(Succeed())
			// json.Indent keeps the whitespace around a document; indentJSON drops it.
			g.Expect(json.Indent(&want, []byte(strings.TrimSpace(doc)), "", "  ")).To(Succeed())
			g.Expect(got.String()).To(Equal(want.String()))
		})
	}
}
