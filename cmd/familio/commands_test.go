package main

import (
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
)

// TestRunWhoami_PrintsAccountIdentity covers the account read end to end: the
// uuid comes from the JWT, the email and display name from GET /profile.
func TestRunWhoami_PrintsAccountIdentity(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.Path).To(Equal("/api/v2/profile"))
		_, _ = io.WriteString(w, `{"user":{"uuid":"`+testOwnerUUID+`","email":"dmalch@gmail.com"},
			"profile":{"displayName":"Мальчиков Д.","firstName":"Дмитрий","lastName":"Мальчиков",
			"middleName":"Сергеевич","gender":"male"}}`)
	})

	code, out, errb := runArgs("-cookies", cookies, "whoami")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring(testOwnerUUID))
	g.Expect(out).To(ContainSubstring("dmalch@gmail.com"))
	g.Expect(out).To(ContainSubstring("Мальчиков Д."))
}

// TestRunWhoami_UnauthenticatedIsCommandError proves a dead session is reported
// rather than printing a half-empty record.
func TestRunWhoami_UnauthenticatedIsCommandError(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	code, _, errb := runArgs("-cookies", cookies, "whoami")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("not logged in"))
}

// TestRunGraph_PrintsTheTreeGraph covers the one-request tree read.
func TestRunGraph_PrintsTheTreeGraph(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.Path).To(Equal("/api/v2/tree"))
		_, _ = io.WriteString(w, `{"nodes":[
			{"nodeId":"p1","nodeParams":{"role":"root","parents":[],"partners":[]}},
			{"nodeId":"p2","nodeParams":{"role":"child",
			 "parents":[{"sex":"male","nodeId":"p1"}],"partners":[]}}]}`)
	})

	code, out, errb := runArgs("-cookies", cookies, "graph")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring(`"nodeId": "p1"`))
	g.Expect(out).To(ContainSubstring(`"nodeId": "p2"`))
	g.Expect(out).To(ContainSubstring(`"sex": "male"`))
}

// TestRunPersonGet_PrintsBasicRelationsAndEvents covers the composite read: the
// basic record, the derived relations, and the birth year taken from the events.
func TestRunPersonGet_PrintsBasicRelationsAndEvents(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/persons/p1/basic":
			_, _ = io.WriteString(w, `{"uuid":"p1","firstName":"Леонтий","lastName":"Тюжин",
				"gender":"male","updatedAt":"2026-07-14T21:37:12+00:00"}`)
		case "/api/v2/persons/p1/events":
			_, _ = io.WriteString(w, `[{"uuid":"e1","type":"birth",
				"date":{"calendar":"gregorian","type":"equal","first":{"year":1890,"type":"gregorian"}},
				"participants":[{"personUuid":"p1","role":"child"},
				                {"personUuid":"p2","role":"parent","displayName":"Епифан"}]}]`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	code, out, errb := runArgs("-cookies", cookies, "person", "get", "p1")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring("Тюжин"))
	g.Expect(out).To(ContainSubstring(`"birthYear": 1890`))
	g.Expect(out).To(ContainSubstring(`"parents"`))
	g.Expect(out).To(ContainSubstring("p2"))
}

// TestRunSettlementPersons_UsesThePublicRead checks the one command that needs no
// credentials at all.
func TestRunSettlementPersons_UsesThePublicRead(t *testing.T) {
	g := NewWithT(t)
	var sawAuth string
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.Path).To(Equal("/api/v2/persons"))
		g.Expect(r.URL.Query().Get("settlement")).To(Equal("s1"))
		sawAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"pager":{"page":1,"itemsPerPage":300,"totalItems":1},
			"data":[{"uuid":"u1","displayName":"Августа Степановна","catalogKey":null}]}`)
	})

	code, out, errb := runArgs("settlement", "persons", "s1")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring("Августа Степановна"))
	g.Expect(sawAuth).To(BeEmpty(), "the settlement list is public — no bearer should be sent")
}

// TestRunSourcesList_PrintsCitations covers the sources read.
func TestRunSourcesList_PrintsCitations(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.Path).To(Equal("/api/v2/persons/p1/sources"))
		_, _ = io.WriteString(w, `[{"uuid":"s1","type":"case","comment":"","name":"Ревизские сказки",
			"requisites":"ГИА … ф. 145 оп. 1 д. 431","years":"1811 - 1811","catalog":null}]`)
	})

	code, out, errb := runArgs("-cookies", cookies, "sources", "list", "p1")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring("Ревизские сказки"))
	g.Expect(out).To(ContainSubstring("1811 - 1811"))
}

// TestRunTree_CrawlsFromTheRoot covers the BFS crawl: the root plus the parent it
// discovers, with -depth bounding the walk.
func TestRunTree_CrawlsFromTheRoot(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/persons/p1/events":
			_, _ = io.WriteString(w, `[{"uuid":"e1","type":"birth",
				"date":{"calendar":"gregorian","type":"equal","first":{"year":1890,"type":"gregorian"}},
				"participants":[{"personUuid":"p1","role":"child","displayName":"Леонтий"},
				                {"personUuid":"p2","role":"parent","displayName":"Епифан"}]}]`)
		case "/api/v2/persons/p2/events":
			_, _ = io.WriteString(w, `[{"uuid":"e2","type":"birth",
				"date":{"calendar":"gregorian","type":"equal","first":null},
				"participants":[{"personUuid":"p2","role":"child","displayName":"Епифан"}]}]`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	code, out, errb := runArgs("-cookies", cookies, "tree", "p1", "-up", "-depth", "1")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring(`"uuid": "p1"`))
	g.Expect(out).To(ContainSubstring(`"uuid": "p2"`))
	g.Expect(out).To(ContainSubstring(`"year": 1890`))
}

// TestRunHistoryFilters_PrintsFacets covers the facet read and its no-positionals
// guard.
func TestRunHistoryFilters_PrintsFacets(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.Method).To(Equal(http.MethodPost))
		g.Expect(r.URL.Path).To(Equal("/api/v2/persons/history/" + testOwnerUUID + "/get-filters-data"))
		_, _ = io.WriteString(w, `{"operationFilter":[{"item":{"value":"update","displayValue":"Изменение"},"count":7}],
			"authorFilter":[{"item":{"value":"`+testOwnerUUID+`","displayValue":"Мальчиков Д."},"count":7}],
			"causeFilter":[],"personDataTypeFilter":[],"personFilter":[],"personFilterHasMore":false}`)
	})

	code, out, errb := runArgs("-cookies", cookies, "history", "filters")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring("Изменение"))
	g.Expect(out).To(ContainSubstring("Мальчиков Д."))
}

func TestRunHistoryFilters_RejectsPositionalArgs(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("history", "filters", "extra")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("takes no positional arguments"))
}

// TestRunMatchesList_AllFollowsTheCursor covers the -all path: it must keep
// scrolling while the cursor reports more, and concatenate every page.
func TestRunMatchesList_AllFollowsTheCursor(t *testing.T) {
	g := NewWithT(t)
	var requests int
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.Path).To(Equal("/api/v2/users/" + testOwnerUUID + "/matches/get-by-filters-scroll"))
		requests++
		switch requests {
		case 1:
			g.Expect(r.URL.Query().Get("pageAfterItem")).To(BeEmpty(), "the first page carries no cursor")
			_, _ = io.WriteString(w, `{"data":[{"uuid":"m1","score":99,"status":"undecided"}],
				"pager":{"lastItem":"m1","hasMore":true},"dataVersionMark":"2026-07-27T08:23:44+00:00"}`)
		case 2:
			g.Expect(r.URL.Query().Get("pageAfterItem")).To(Equal("m1"), "the cursor must be passed on")
			_, _ = io.WriteString(w, `{"data":[{"uuid":"m2","score":3,"status":"rejected"}],
				"pager":{"lastItem":"m2","hasMore":false}}`)
		default:
			t.Errorf("scrolled past the end (request %d)", requests)
		}
	})

	code, out, errb := runArgs("-cookies", cookies, "matches", "list", "-all")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(requests).To(Equal(2))
	g.Expect(out).To(ContainSubstring(`"uuid": "m1"`))
	g.Expect(out).To(ContainSubstring(`"uuid": "m2"`))
}

// TestRunCommand_HelpFlagIsNotAnError covers the -h short-circuit shared by every
// leaf command: the FlagSet prints usage and the command exits 0.
func TestRunCommand_HelpFlagIsNotAnError(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("person", "get", "-h")
	g.Expect(code).To(Equal(0))
	g.Expect(errb).To(ContainSubstring("-cookies"))
}
