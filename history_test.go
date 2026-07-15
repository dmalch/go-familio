package familio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// historyOwner is the account uuid embedded in the test JWT; the history
// endpoints address the owner in the path, resolved via AccountUUID.
const historyOwner = "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0"

// historyTestJWT builds an unsigned JWT whose payload carries a far-future exp
// and the historyOwner uuid claim, so AccountUUID resolves without a network
// round-trip beyond the scrape.
func historyTestJWT() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"exp":9999999999,"uuid":"` + historyOwner + `"}`))
	return header + "." + payload + ".sig"
}

// historyTestServer is biographyTestServer with a uuid-bearing token, so the
// history endpoints can build their owner-addressed paths.
func historyTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"`+historyTestJWT()+`"}}</script>`)
			return
		}
		handler(w, r)
	}))
}

// historyListFixture is a trimmed live response of the list endpoint: one
// event delete and one biography update, with the real envelope and record
// shapes (see API.md "Change history sub-resource").
const historyListFixture = `{
  "data": [
    {"record": {"id": 112528756, "happenedAt": "2026-07-14T22:08:21.843518+00:00",
       "cause": "user", "operation": "delete", "personDataBlock": "event",
       "changes": {"type": "birth", "uuid": "ef8210b4-ed8f-4620-bfbe-54af48ae7d02", "comment": "",
         "date": {"type": "equal", "first": null, "second": null, "calendar": "gregorian", "formatted": "Неизвестно"},
         "settlement": null,
         "participants": [{"role": "child", "gender": "male",
           "personUuid": "a01a51d0-b736-493b-8b3f-8096ac20a07c", "displayName": "Мужев АкцТест"}]}},
     "person": {"id": "a01a51d0-b736-493b-8b3f-8096ac20a07c", "lastName": "Мужев", "firstName": "АкцТест",
       "middleName": null, "birthLastName": null, "birthFirstName": null, "gender": "male"},
     "author": {"id": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0", "displayName": "Мальчиков Д."}},
    {"record": {"id": 112525536, "happenedAt": "2026-07-14T21:37:12.860280+00:00",
       "cause": "user", "operation": "update", "personDataBlock": "biography",
       "changes": {"text": "МК Журавкино 1914: жених"}},
     "person": {"id": "15acfcd4-90b2-4633-a50e-04f1bc57d850", "lastName": "Тюжин", "firstName": "Леонтий",
       "middleName": "Епифанович", "birthLastName": null, "birthFirstName": null, "gender": "male"},
     "author": {"id": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0", "displayName": "Мальчиков Д."}}
  ],
  "pager": {"page": 1, "itemsPerPage": 20, "totalItems": 4695}
}`

// TestListPersonsHistory locks the list contract: an owner-addressed GET with
// the mandatory paging/sort params, decoding the {data, pager} envelope with
// block-shaped raw changes.
func TestListPersonsHistory(t *testing.T) {
	RegisterTestingT(t)
	var gotQuery url.Values
	srv := historyTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodGet))
		Expect(r.URL.Path).To(Equal("/api/v2/persons/history/" + historyOwner))
		Expect(r.Header.Get("Authorization")).To(HavePrefix("Bearer eyJ"))
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, historyListFixture)
	})
	defer srv.Close()

	page, err := newHistoryClient(srv).ListPersonsHistory(context.Background(), HistoryFilter{})
	Expect(err).ToNot(HaveOccurred())

	// The API rejects requests missing any of these four params with a 400.
	Expect(gotQuery.Get("page")).To(Equal("1"))
	Expect(gotQuery.Get("itemsPerPage")).To(Equal("20"))
	Expect(gotQuery.Get("orderBy")).To(Equal("id"))
	Expect(gotQuery.Get("orderDirection")).To(Equal("desc"))

	Expect(page.Pager.TotalItems).To(Equal(4695))
	Expect(page.Data).To(HaveLen(2))

	first := page.Data[0]
	Expect(first.Record.ID).To(Equal(int64(112528756)))
	Expect(first.Record.Operation).To(Equal("delete"))
	Expect(first.Record.PersonDataBlock).To(Equal("event"))
	Expect(first.Record.Cause).To(Equal("user"))
	Expect(first.Person.LastName).To(Equal("Мужев"))
	Expect(first.Author.DisplayName).To(Equal("Мальчиков Д."))

	// The changes snapshot stays raw but must round-trip as the block's shape.
	var eventChanges struct {
		Type string `json:"type"`
		UUID string `json:"uuid"`
	}
	Expect(json.Unmarshal(first.Record.Changes, &eventChanges)).To(Succeed())
	Expect(eventChanges.Type).To(Equal("birth"))

	var bioChanges struct {
		Text string `json:"text"`
	}
	Expect(json.Unmarshal(page.Data[1].Record.Changes, &bioChanges)).To(Succeed())
	Expect(bioChanges.Text).To(Equal("МК Журавкино 1914: жених"))
}

// TestHistoryFilterQuery locks the wire encoding of every filter: bracketed
// array params, indexed personDataType triples, and the RFC3339 date range.
func TestHistoryFilterQuery(t *testing.T) {
	RegisterTestingT(t)
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	till := time.Date(2026, 7, 14, 23, 59, 59, 0, time.UTC)
	f := HistoryFilter{
		Text:       "Тюжин",
		Operations: []string{"create", "update"},
		Causes:     []string{"initialization"},
		AuthorIDs:  []string{"a-1"},
		PersonIDs:  []string{"p-1", "p-2"},
		DataTypes: []HistoryDataType{
			{PersonDataBlock: "event", EventType: "birth"},
			{PersonDataBlock: "source", SourceType: "case"},
		},
		From:         from,
		Till:         till,
		Page:         3,
		ItemsPerPage: 50,
		Ascending:    true,
	}

	q := f.query()
	Expect(q.Get("page")).To(Equal("3"))
	Expect(q.Get("itemsPerPage")).To(Equal("50"))
	Expect(q.Get("orderBy")).To(Equal("id"))
	Expect(q.Get("orderDirection")).To(Equal("asc"))
	Expect(q.Get("text")).To(Equal("Тюжин"))
	Expect(q["operation[]"]).To(Equal([]string{"create", "update"}))
	Expect(q["cause[]"]).To(Equal([]string{"initialization"}))
	Expect(q["authorId[]"]).To(Equal([]string{"a-1"}))
	Expect(q["personId[]"]).To(Equal([]string{"p-1", "p-2"}))
	Expect(q.Get("date[from]")).To(Equal("2026-07-01T00:00:00Z"))
	Expect(q.Get("date[till]")).To(Equal("2026-07-14T23:59:59Z"))
	Expect(q.Get("personDataType[0][personDataBlock]")).To(Equal("event"))
	Expect(q.Get("personDataType[0][eventType]")).To(Equal("birth"))
	Expect(q.Get("personDataType[0][sourceType]")).To(BeEmpty())
	Expect(q.Get("personDataType[1][personDataBlock]")).To(Equal("source"))
	Expect(q.Get("personDataType[1][sourceType]")).To(Equal("case"))
}

// historyFiltersFixture is a trimmed live get-filters-data response.
const historyFiltersFixture = `{
  "authorFilter": [
    {"item": {"value": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0", "displayValue": "Мальчиков Д."}, "count": 3875},
    {"item": {"value": "00000000-0000-0000-0000-000000000000", "displayValue": ""}, "count": 820}],
  "operationFilter": [
    {"item": {"value": "create", "displayValue": "Добавление"}, "count": 3512},
    {"item": {"value": "update", "displayValue": "Изменение"}, "count": 505},
    {"item": {"value": "delete", "displayValue": "Удаление"}, "count": 678}],
  "personDataTypeFilter": [
    {"item": {"value": {"personDataBlock": "basic", "eventType": null, "sourceType": null},
      "displayValue": {"type": "Основные данные персоны", "subtype": null}}, "count": 1308},
    {"item": {"value": {"personDataBlock": "event", "eventType": "birth", "sourceType": null},
      "displayValue": {"type": "Событие", "subtype": "Рождение"}}, "count": 1665},
    {"item": {"value": {"personDataBlock": "source", "eventType": null, "sourceType": "case"},
      "displayValue": {"type": "Источник", "subtype": "Архивный документ Дело"}}, "count": 32}],
  "personFilter": [
    {"item": {"value": "a01a51d0-b736-493b-8b3f-8096ac20a07c", "displayValue": "Мужев АкцТест"}, "count": 8}],
  "personFilterHasMore": true,
  "causeFilter": [
    {"item": {"value": "user", "displayValue": "Пользователь"}, "count": 3875},
    {"item": {"value": "initialization", "displayValue": "Инициализация"}, "count": 820}]
}`

// TestGetHistoryFilters locks the facets contract: an owner-addressed POST
// with an empty JSON body, decoding every facet family.
func TestGetHistoryFilters(t *testing.T) {
	RegisterTestingT(t)
	srv := historyTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPost))
		Expect(r.URL.Path).To(Equal("/api/v2/persons/history/" + historyOwner + "/get-filters-data"))
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(body)).To(Equal("{}"))
		_, _ = io.WriteString(w, historyFiltersFixture)
	})
	defer srv.Close()

	filters, err := newHistoryClient(srv).GetHistoryFilters(context.Background())
	Expect(err).ToNot(HaveOccurred())

	Expect(filters.Operations).To(HaveLen(3))
	Expect(filters.Operations[0].Item.Value).To(Equal("create"))
	Expect(filters.Operations[0].Count).To(Equal(3512))

	Expect(filters.Authors[1].Item.Value).To(Equal("00000000-0000-0000-0000-000000000000"))
	Expect(filters.Causes[1].Item.Value).To(Equal("initialization"))
	Expect(filters.PersonsHasMore).To(BeTrue())
	Expect(filters.Persons[0].Item.DisplayValue).To(Equal("Мужев АкцТест"))

	birth := filters.DataTypes[1]
	Expect(birth.Item.Value.PersonDataBlock).To(Equal("event"))
	Expect(*birth.Item.Value.EventType).To(Equal("birth"))
	Expect(birth.Item.Value.SourceType).To(BeNil())
	Expect(*birth.Item.DisplayValue.Subtype).To(Equal("Рождение"))
}

func newHistoryClient(srv *httptest.Server) *Client {
	c, _ := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookiesFromHeader("t=secret"),
		RateLimit: 1000,
	})
	return c
}
