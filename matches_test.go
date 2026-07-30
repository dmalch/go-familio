package familio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	. "github.com/onsi/gomega"
)

// matchesListFixture is a trimmed live get-by-filters response carrying one
// match of each foreignPerson variant: a regularPerson from another user's tree
// and a catalogPerson from a record catalog (see API.md "Matches
// sub-resource").
const matchesListFixture = `{
  "data": [
    {"uuid": "95e794df-4edb-4661-8bd9-61fd3fc6d52e", "score": 99, "date": "2026-07-20",
     "ownPerson": {"ownerId": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0", "gender": "male",
       "birthPlace": {"uuid": "e0c1a09c-b7ed-4d5c-a22f-3a86db42bbc6", "primaryName": "Журавкино",
         "additionalNames": ["Жеравкина"],
         "mainGeorequisite": {"level1": "Республика Мордовия", "level2": "Зубово-Полянский муниципальный район", "year": 2019},
         "type": "село", "status": "жилой",
         "coordinate": {"type": "Point", "coordinates": [42.578453, 54.126934]}},
       "deathPlace": null, "photo": null, "biography": "", "isMine": true, "isMe": false,
       "canBuildTree": true, "privacyType": "visible_for_all", "updatedAt": "2025-01-08T08:57:20+00:00",
       "tags": [], "isGrantedToMe": true, "uuid": "5d1f1220-b8fe-49b5-8726-0552226ebe19",
       "type": "regularPerson", "shortDisplayName": "Сандин Яков Перфилович",
       "displayName": "Сандин Яков Перфилович", "birthSettlementText": "",
       "birthDate": {"type": "equal", "calendar": "gregorian", "formatted": "21.11.1837"},
       "deathDate": {"type": "equal", "calendar": "gregorian", "formatted": "Неизвестно"},
       "hasDeathEvent": true},
     "foreignPerson": {"ownerId": "34dc8948-c636-4e1a-a228-d16cb4d71085", "gender": "male",
       "deathPlace": null, "photo": null, "biography": "", "isMine": false, "isMe": false,
       "canBuildTree": true, "privacyType": "visible_for_all", "updatedAt": "2026-07-13T10:56:19+00:00",
       "tags": [], "isGrantedToMe": false, "uuid": "59ef9b6c-1a43-45b8-bfa4-2fd923651553",
       "type": "regularPerson", "shortDisplayName": "Сандин Яков Перфилов",
       "displayName": "Сандин Яков Перфилов", "birthSettlementText": "",
       "birthDate": {"type": "equal", "calendar": "gregorian", "formatted": "21.11.1837"},
       "deathDate": {"type": "after", "calendar": "gregorian", "formatted": "После 1861"},
       "hasDeathEvent": true},
     "detailedScore": {"firstName": 20, "lastName": 20, "middleName": 20,
       "birthDate": 10, "deathDate": 0, "birthPlace": 10},
     "status": "undecided"},
    {"uuid": "b028951e-1554-45ce-969f-f268326516b3", "score": 3, "date": "2026-07-20",
     "ownPerson": {"ownerId": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0", "gender": "male",
       "birthPlace": null, "deathPlace": null, "isMine": true, "uuid": "245e04ce-8e00-4fd7-a7ce-72752c817795",
       "type": "regularPerson", "shortDisplayName": "Гусев Александр Иванович",
       "displayName": "Гусев Александр Иванович", "biography": "1-я всеобщая перепись Нижняя Верея 1897: семья 2",
       "birthDate": null, "deathDate": null, "hasDeathEvent": false},
     "foreignPerson": {"catalogKey": "vss", "catalogName": "Ведомости справок о судимости 1870–1927 годов",
       "updatedAt": "2026-07-20T00:00:00+00:00", "updating": false,
       "uuid": "c34c2eb5-1685-48aa-9e28-d0f5a7ff5789", "type": "catalogPerson",
       "shortDisplayName": "Гусев Александр Иванов", "displayName": "Гусев Александр Иванов",
       "birthSettlementText": "", "birthDate": "1890", "deathDate": null, "hasDeathEvent": false},
     "detailedScore": {"firstName": 20, "lastName": 20, "middleName": 20,
       "birthDate": 10, "deathDate": 0, "birthPlace": 0},
     "status": "undecided"}
  ],
  "pager": {"page": 1, "itemsPerPage": 15, "totalItems": 685},
  "dataVersionMark": "2026-07-27T08:23:44+00:00"
}`

// TestListMatches locks the list contract: an owner-addressed POST carrying the
// filter as its body, and decoding of both foreignPerson variants.
func TestListMatches(t *testing.T) {
	RegisterTestingT(t)
	var gotQuery url.Values
	var gotBody []byte
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPost))
		Expect(r.URL.Path).To(Equal("/api/v2/users/" + testOwnerUUID + "/matches/get-by-filters"))
		Expect(r.Header.Get("Authorization")).To(HavePrefix("Bearer eyJ"))
		gotQuery = r.URL.Query()
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		gotBody = body
		_, _ = io.WriteString(w, matchesListFixture)
	})
	defer srv.Close()

	page, err := newTestClient(srv).ListMatches(context.Background(), MatchFilter{})
	Expect(err).ToNot(HaveOccurred())

	Expect(gotQuery.Get("page")).To(Equal("1"))
	Expect(gotQuery.Get("itemsPerPage")).To(Equal("15"))

	// The filter travels in the body, with all seven keys always present.
	var sent map[string]any
	Expect(json.Unmarshal(gotBody, &sent)).To(Succeed())
	Expect(sent).To(HaveLen(7))
	Expect(sent["status"]).To(BeEmpty())
	Expect(sent["minTotalScore"]).To(BeEquivalentTo(1))
	Expect(sent["maxTotalScore"]).To(BeEquivalentTo(99))

	Expect(page.Pager.TotalItems).To(Equal(685))
	Expect(page.DataVersionMark).To(Equal("2026-07-27T08:23:44+00:00"))
	Expect(page.Data).To(HaveLen(2))

	// A tree-to-tree match: both sides are regular persons with owners.
	tree := page.Data[0]
	Expect(tree.UUID).To(Equal("95e794df-4edb-4661-8bd9-61fd3fc6d52e"))
	Expect(tree.Score).To(Equal(99))
	Expect(tree.Date).To(Equal("2026-07-20"))
	Expect(tree.Status).To(Equal(MatchStatusUndecided))
	Expect(tree.DetailedScore).To(Equal(MatchScore{
		FirstName: 20, LastName: 20, MiddleName: 20, BirthDate: 10, DeathDate: 0, BirthPlace: 10,
	}))
	Expect(tree.OwnPerson.Type).To(Equal("regularPerson"))
	Expect(tree.OwnPerson.DisplayName).To(Equal("Сандин Яков Перфилович"))
	Expect(tree.OwnPerson.IsMine).To(BeTrue())
	Expect(tree.OwnPerson.OwnerID).To(Equal(testOwnerUUID))
	Expect(tree.OwnPerson.CatalogKey).To(BeNil())
	Expect(tree.OwnPerson.BirthPlace.PrimaryName).To(Equal("Журавкино"))
	Expect(tree.OwnPerson.BirthPlace.MainGeorequisite.Level1).To(Equal("Республика Мордовия"))
	Expect(tree.ForeignPerson.OwnerID).To(Equal("34dc8948-c636-4e1a-a228-d16cb4d71085"))
	Expect(tree.ForeignPerson.IsMine).To(BeFalse())
	Expect(tree.ForeignPerson.BirthPlace).To(BeNil())

	// birthDate arrives as an object here and as a plain string on the catalog
	// side; FlexDate absorbs both.
	Expect(tree.ForeignPerson.BirthDate.Formatted).To(Equal("21.11.1837"))
	Expect(tree.ForeignPerson.DeathDate.Formatted).To(Equal("После 1861"))

	// A catalog match: the foreign side is a record, not a person in a tree.
	catalog := page.Data[1]
	Expect(catalog.Score).To(Equal(3))
	Expect(catalog.ForeignPerson.Type).To(Equal("catalogPerson"))
	Expect(*catalog.ForeignPerson.CatalogKey).To(Equal("vss"))
	Expect(catalog.ForeignPerson.CatalogName).To(Equal("Ведомости справок о судимости 1870–1927 годов"))
	Expect(catalog.ForeignPerson.OwnerID).To(BeEmpty())
	Expect(catalog.ForeignPerson.PrivacyType).To(BeEmpty())
	Expect(catalog.ForeignPerson.BirthDate.Formatted).To(Equal("1890"))
	Expect(catalog.ForeignPerson.DeathDate.Present).To(BeFalse())
	Expect(catalog.OwnPerson.Biography).To(HavePrefix("1-я всеобщая перепись"))
}

// TestMatchFilterBody locks the body encoding: every key present even when
// empty (the API 400s on a missing one), empty lists as [] not null, and the
// score-window defaults.
func TestMatchFilterBody(t *testing.T) {
	RegisterTestingT(t)

	zero, err := json.Marshal(MatchFilter{}.body())
	Expect(err).ToNot(HaveOccurred())
	Expect(string(zero)).To(Equal(
		`{"person":[],"user":[],"catalog":[],"date":[],"status":[],"minTotalScore":1,"maxTotalScore":99}`))

	full, err := json.Marshal(MatchFilter{
		PersonIDs: []string{"p-1", "p-2"},
		UserIDs:   []string{"u-1"},
		Catalogs:  []string{"vss", "blockade"},
		Dates:     []string{"2026-07-20"},
		Statuses:  []string{MatchStatusUndecided, MatchStatusConfirmed},
		MinScore:  90,
		MaxScore:  95,
	}.body())
	Expect(err).ToNot(HaveOccurred())
	Expect(string(full)).To(Equal(`{"person":["p-1","p-2"],"user":["u-1"],` +
		`"catalog":["vss","blockade"],"date":["2026-07-20"],` +
		`"status":["undecided","confirmed"],"minTotalScore":90,"maxTotalScore":95}`))

	// Paging travels in the query, not the body.
	q := MatchFilter{Page: 3, ItemsPerPage: 50}.query()
	Expect(q.Get("page")).To(Equal("3"))
	Expect(q.Get("itemsPerPage")).To(Equal("50"))
}

// TestScrollMatches locks the cursor contract: pageAfterItem in the query and
// the {lastItem, hasMore} pager.
func TestScrollMatches(t *testing.T) {
	RegisterTestingT(t)
	var gotQuery url.Values
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPost))
		Expect(r.URL.Path).To(Equal("/api/v2/users/" + testOwnerUUID + "/matches/get-by-filters-scroll"))
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, `{"data": [], "dataVersionMark": "2026-07-27T08:23:44+00:00",
		  "pager": {"lastItem": "3875136a-bacc-4aab-88f4-c6e812cef4ec", "hasMore": true}}`)
	})
	defer srv.Close()
	c := newTestClient(srv)

	page, err := c.ScrollMatches(context.Background(), MatchFilter{ItemsPerPage: 2}, "b5cc90fb-2429-4fac-83e7-6c3f5b8686a2")
	Expect(err).ToNot(HaveOccurred())
	Expect(gotQuery.Get("itemsPerPage")).To(Equal("2"))
	Expect(gotQuery.Get("pageAfterItem")).To(Equal("b5cc90fb-2429-4fac-83e7-6c3f5b8686a2"))
	Expect(page.Pager.LastItem).To(Equal("3875136a-bacc-4aab-88f4-c6e812cef4ec"))
	Expect(page.Pager.HasMore).To(BeTrue())

	// The first page carries no cursor at all.
	_, err = c.ScrollMatches(context.Background(), MatchFilter{}, "")
	Expect(err).ToNot(HaveOccurred())
	Expect(gotQuery).ToNot(HaveKey("pageAfterItem"))
	Expect(gotQuery.Get("itemsPerPage")).To(Equal("15"))
}

// matchesFiltersFixture is a trimmed live get-filters-data response.
const matchesFiltersFixture = `{
  "dateFilter": [
    {"item": {"value": "2026-07-20", "displayValue": "20 июля "}, "count": 7},
    {"item": {"value": "2026-07-06", "displayValue": "6 июля "}, "count": 396}],
  "personFilter": [
    {"item": {"value": "826fa6f6-62d7-4d4d-839f-41991e933563", "displayValue": "Абмашкин Косьма Иванович"}, "count": 1}],
  "userFilter": [
    {"item": {"value": "988a95c2-afe7-49c1-a158-eee5bd6ab206", "displayValue": "Акимова Арина"}, "count": 125}],
  "catalogFilter": [
    {"item": {"value": "vss", "displayValue": "Ведомости справок о судимости 1870–1927 годов"}, "count": 15},
    {"item": {"value": "blockade", "displayValue": "Книги памяти Блокады Ленинграда"}, "count": 4}],
  "statusFilter": [
    {"item": {"value": "rejected", "displayValue": "Отклонённые"}, "count": 86},
    {"item": {"value": "undecided", "displayValue": "Ожидающие"}, "count": 376},
    {"item": {"value": "confirmed", "displayValue": "Подтверждённые"}, "count": 223}]
}`

// TestGetMatchFilters locks the facets contract: an owner-addressed POST
// carrying the filter, decoding every facet family.
func TestGetMatchFilters(t *testing.T) {
	RegisterTestingT(t)
	var gotBody []byte
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPost))
		Expect(r.URL.Path).To(Equal("/api/v2/users/" + testOwnerUUID + "/matches/get-filters-data"))
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		gotBody = body
		_, _ = io.WriteString(w, matchesFiltersFixture)
	})
	defer srv.Close()

	filters, err := newTestClient(srv).GetMatchFilters(context.Background(),
		MatchFilter{Statuses: []string{MatchStatusConfirmed}})
	Expect(err).ToNot(HaveOccurred())
	Expect(string(gotBody)).To(ContainSubstring(`"status":["confirmed"]`))

	Expect(filters.Statuses).To(HaveLen(3))
	Expect(filters.Statuses[1].Item.Value).To(Equal(MatchStatusUndecided))
	Expect(filters.Statuses[1].Item.DisplayValue).To(Equal("Ожидающие"))
	Expect(filters.Statuses[1].Count).To(Equal(376))
	Expect(filters.Dates[0].Item.Value).To(Equal("2026-07-20"))
	Expect(filters.Catalogs[0].Item.Value).To(Equal("vss"))
	Expect(filters.Catalogs[0].Item.DisplayValue).To(HavePrefix("Ведомости справок"))
	Expect(filters.Users[0].Item.DisplayValue).To(Equal("Акимова Арина"))
	Expect(filters.Persons[0].Item.DisplayValue).To(Equal("Абмашкин Косьма Иванович"))
}

// TestDecideMatches locks the status-write contract: one path per verb and a
// bare JSON array of match uuids as the body.
func TestDecideMatches(t *testing.T) {
	RegisterTestingT(t)
	ids := []string{"95e794df-4edb-4661-8bd9-61fd3fc6d52e", "b028951e-1554-45ce-969f-f268326516b3"}

	for _, tc := range []struct {
		name string
		call func(*Client) error
		path string
	}{
		{"confirm", func(c *Client) error { return c.ConfirmMatches(context.Background(), ids) }, "confirm-by-ids"},
		{"reject", func(c *Client) error { return c.RejectMatches(context.Background(), ids) }, "reject-by-ids"},
		{"undecide", func(c *Client) error { return c.UndecideMatches(context.Background(), ids) }, "undecide-by-ids"},
	} {
		var gotPath string
		var gotBody []byte
		srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost))
			gotPath = r.URL.Path
			body, err := io.ReadAll(r.Body)
			Expect(err).ToNot(HaveOccurred())
			gotBody = body
			w.WriteHeader(http.StatusOK)
		})

		Expect(tc.call(newTestClient(srv))).To(Succeed(), tc.name)
		Expect(gotPath).To(Equal("/api/v2/users/" + testOwnerUUID + "/matches/" + tc.path))
		// A bare array, not an object wrapping one.
		Expect(string(gotBody)).To(Equal(
			`["95e794df-4edb-4661-8bd9-61fd3fc6d52e","b028951e-1554-45ce-969f-f268326516b3"]`))
		srv.Close()
	}
}

// TestDecideMatchesRejectsEmpty checks that an empty id list fails locally
// rather than posting an empty array.
func TestDecideMatchesRejectsEmpty(t *testing.T) {
	RegisterTestingT(t)
	called := false
	srv := authedTestServer(func(http.ResponseWriter, *http.Request) { called = true })
	defer srv.Close()
	c := newTestClient(srv)

	Expect(c.ConfirmMatches(context.Background(), nil)).To(MatchError(ContainSubstring("no match uuids")))
	Expect(c.RejectMatches(context.Background(), []string{})).To(MatchError(ContainSubstring("no match uuids")))
	Expect(c.UndecideMatches(context.Background(), nil)).To(MatchError(ContainSubstring("no match uuids")))
	Expect(called).To(BeFalse())
}
