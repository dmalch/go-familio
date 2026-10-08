package familio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	. "github.com/onsi/gomega"
)

// wireSearchPage is a real GET /api/v3/persons?lastName=Мальчиков answer from
// 2026-10-08, trimmed to one regular person (another account's public profile)
// and one catalog record, \u escapes kept as familio sends them.
const wireSearchPage = `{"pager":{"page":1,"itemsPerPage":2,"totalItems":1253},"data":[{"settlementEvents":[],"ownerId":"9812dabd-4acf-4891-9fc1-b9a44a203cd2","gender":"male","birthPlace":{"uuid":"c99f7e4a-0940-46fd-af18-d5f5a2efb81c","primaryName":"\u041b\u0435\u0434\u043d\u0435\u0432\u043e","additionalNames":[],"mainGeorequisite":{"level1":"\u041b\u0435\u043d\u0438\u043d\u0433\u0440\u0430\u0434\u0441\u043a\u0430\u044f \u043e\u0431\u043b\u0430\u0441\u0442\u044c","level2":"\u041a\u0438\u0440\u043e\u0432\u0441\u043a\u0438\u0439 \u043c\u0443\u043d\u0438\u0446\u0438\u043f\u0430\u043b\u044c\u043d\u044b\u0439 \u0440\u0430\u0439\u043e\u043d","year":2019},"type":"\u0434\u0435\u0440\u0435\u0432\u043d\u044f","status":"\u0436\u0438\u043b\u043e\u0439","coordinate":{"type":"Point","coordinates":[31.534664,60.074471]}},"deathPlace":null,"deathSettlementText":"","photo":null,"biography":"","isMine":false,"isMe":false,"canBuildTree":true,"privacyType":"visible_for_all","updatedAt":"2025-09-09T12:58:50+00:00","tags":[],"isGrantedToMe":false,"uuid":"982b57c4-970b-4d26-a7dc-ee6ab07c0aae","type":"regularPerson","shortDisplayName":"\u041c\u0430\u043b\u044c\u0447\u0438\u0448\u043a\u043e\u0432 \u041c\u0430\u043b\u044c\u0447\u0438\u043a\u043e\u0432 \u0412\u0430\u0441\u0438\u043b\u0438\u0439 \u0410\u043b\u0435\u043a\u0441\u0435\u0435\u0432\u0438\u0447","displayName":"\u041c\u0430\u043b\u044c\u0447\u0438\u0448\u043a\u043e\u0432 \u041c\u0430\u043b\u044c\u0447\u0438\u043a\u043e\u0432 \u0412\u0430\u0441\u0438\u043b\u0438\u0439 \u0410\u043b\u0435\u043a\u0441\u0435\u0435\u0432\u0438\u0447","originalDisplayName":"\u041c\u0430\u043b\u044c\u0447\u0438\u0448\u043a\u043e\u0432 \u041c\u0430\u043b\u044c\u0447\u0438\u043a\u043e\u0432 \u0412\u0430\u0441\u0438\u043b\u0438\u0439 \u0410\u043b\u0435\u043a\u0441\u0435\u0435\u0432\u0438\u0447","birthSettlementText":"","birthDate":{"type":"equal","calendar":"gregorian","first":{"year":1892,"month":null,"day":null,"formatted":"1892","type":"gregorian"},"second":null,"formatted":"1892"},"deathDate":{"type":"equal","calendar":"gregorian","first":null,"second":null,"formatted":"\u041d\u0435\u0438\u0437\u0432\u0435\u0441\u0442\u043d\u043e"},"hasDeathEvent":true},{"catalogKey":"gwarmil","catalogName":"\u0421\u043f\u0438\u0441\u043a\u0438 \u0443\u0447\u0430\u0441\u0442\u043d\u0438\u043a\u043e\u0432 \u041f\u0435\u0440\u0432\u043e\u0439 \u041c\u0438\u0440\u043e\u0432\u043e\u0439 \u0432\u043e\u0439\u043d\u044b (\u043f\u0440\u043e\u0435\u043a\u0442 \u041f\u0430\u043c\u044f\u0442\u0438 \u0433\u0435\u0440\u043e\u0435\u0432 \u0412\u0435\u043b\u0438\u043a\u043e\u0439 \u0432\u043e\u0439\u043d\u044b 1914\u20131918)","updatedAt":"2023-11-28T17:45:48+00:00","updating":false,"mentions":[],"uuid":"774b6dcb-b44a-4304-944b-d7a5d4513c61","type":"catalogPerson","shortDisplayName":"\u041c\u0430\u043b\u044c\u0447\u0438\u043a\u043e\u0432 \u041c\u0438\u0445\u0430\u0438\u043b ","displayName":"\u041c\u0430\u043b\u044c\u0447\u0438\u043a\u043e\u0432 \u041c\u0438\u0445\u0430\u0438\u043b ","originalDisplayName":"\u041c\u0430\u043b\u044c\u0447\u0438\u043a\u043e\u0432 \u041c\u0438\u0445\u0430\u0438\u043b ","birthSettlementText":"","birthDate":null,"deathDate":null,"hasDeathEvent":false}]}`

func ptrInt(v int) *int { return &v }

// TestPersonSearchQueryDefaults pins the parameters familio rejects a search
// without: paging, the sort and mentions are always sent.
func TestPersonSearchQueryDefaults(t *testing.T) {
	RegisterTestingT(t)

	q, err := PersonSearch{LastName: "Мальчиков"}.query()

	Expect(err).ToNot(HaveOccurred())
	Expect(q).To(Equal(url.Values{
		"lastName":       {"Мальчиков"},
		"page":           {"1"},
		"itemsPerPage":   {"20"},
		"orderBy":        {"score"},
		"orderDirection": {"desc"},
		"mentions":       {"false"},
	}))
}

// TestPersonSearchQueryFields covers every filter field's wire name.
func TestPersonSearchQueryFields(t *testing.T) {
	RegisterTestingT(t)

	q, err := PersonSearch{
		LastName:                "Мальчиков",
		LastNameExact:           true,
		FirstAndMiddleName:      "Иван",
		FirstAndMiddleNameExact: true,
		Text:                    "Мальчиков Иван",
		Types:                   []string{PersonSearchCatalog, PersonSearchOtherUsers},
		Gender:                  GenderFemale,
		OrderBy:                 SearchOrderBirth,
		Ascending:               true,
		Page:                    3,
		ItemsPerPage:            50,
	}.query()

	Expect(err).ToNot(HaveOccurred())
	Expect(q).To(Equal(url.Values{
		"lastName":                     {"Мальчиков"},
		"lastNameExactMatch":           {"true"},
		"firstAndMiddleName":           {"Иван"},
		"firstAndMiddleNameExactMatch": {"true"},
		"name":                         {"Мальчиков Иван"},
		"types[0]":                     {"catalog_persons"},
		"types[1]":                     {"other_users_persons"},
		"gender":                       {"female"},
		"page":                         {"3"},
		"itemsPerPage":                 {"50"},
		"orderBy":                      {"min_birth_date"},
		"orderDirection":               {"asc"},
		"mentions":                     {"false"},
	}))
}

// TestPersonSearchQueryDates covers how a DateRange becomes familio's date
// filter. A single date is [equal] with the parts that are set; a range needs
// every part of both bounds, so missing ones widen to the whole year or month,
// and an open side becomes year 1 or 9999, as the web UI sends.
func TestPersonSearchQueryDates(t *testing.T) {
	for _, tc := range []struct {
		name string
		date *DateRange
		want url.Values
	}{
		{"year", &DateRange{Year: 1892}, url.Values{
			"birthDate[calendar]": {"gregorian"}, "birthDate[equal][year]": {"1892"},
		}},
		{"full date, julian", &DateRange{Year: 1892, Month: ptrInt(5), Day: ptrInt(3), Calendar: "julian"}, url.Values{
			"birthDate[calendar]":     {"julian"},
			"birthDate[equal][year]":  {"1892"},
			"birthDate[equal][month]": {"5"},
			"birthDate[equal][day]":   {"3"},
		}},
		{"between years", &DateRange{Year: 1850, Range: RangeBetween, EndYear: ptrInt(1860)}, url.Values{
			"birthDate[calendar]":   {"gregorian"},
			"birthDate[from][year]": {"1850"}, "birthDate[from][month]": {"1"}, "birthDate[from][day]": {"1"},
			"birthDate[till][year]": {"1860"}, "birthDate[till][month]": {"12"}, "birthDate[till][day]": {"31"},
		}},
		{"between months ends on the month's last day", &DateRange{Year: 1900, Month: ptrInt(2), Range: RangeBetween, EndYear: ptrInt(1900), EndMonth: ptrInt(2)}, url.Values{
			"birthDate[calendar]":   {"gregorian"},
			"birthDate[from][year]": {"1900"}, "birthDate[from][month]": {"2"}, "birthDate[from][day]": {"1"},
			"birthDate[till][year]": {"1900"}, "birthDate[till][month]": {"2"}, "birthDate[till][day]": {"28"},
		}},
		{"after", &DateRange{Year: 1900, Range: RangeAfter}, url.Values{
			"birthDate[calendar]":   {"gregorian"},
			"birthDate[from][year]": {"1900"}, "birthDate[from][month]": {"1"}, "birthDate[from][day]": {"1"},
			"birthDate[till][year]": {"9999"}, "birthDate[till][month]": {"12"}, "birthDate[till][day]": {"31"},
		}},
		{"before", &DateRange{Year: 1900, Range: RangeBefore}, url.Values{
			"birthDate[calendar]":   {"gregorian"},
			"birthDate[from][year]": {"1"}, "birthDate[from][month]": {"1"}, "birthDate[from][day]": {"1"},
			"birthDate[till][year]": {"1900"}, "birthDate[till][month]": {"12"}, "birthDate[till][day]": {"31"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			q, err := PersonSearch{Born: tc.date}.query()
			Expect(err).ToNot(HaveOccurred())
			for _, k := range []string{"page", "itemsPerPage", "orderBy", "orderDirection", "mentions"} {
				q.Del(k)
			}
			Expect(q).To(Equal(tc.want))
		})
	}

	t.Run("death dates use their own prefix", func(t *testing.T) {
		RegisterTestingT(t)
		q, err := PersonSearch{Died: &DateRange{Year: 1942}}.query()
		Expect(err).ToNot(HaveOccurred())
		Expect(q.Get("deathDate[equal][year]")).To(Equal("1942"))
		Expect(q.Has("birthDate[calendar]")).To(BeFalse())
	})

	t.Run("rejects what search cannot express", func(t *testing.T) {
		RegisterTestingT(t)
		for _, d := range []*DateRange{
			{Year: 1892, Circa: true},
			{Year: 1850, Range: RangeBetween},
			{Year: 0},
			{Year: 1850, Range: "sometime"},
		} {
			_, err := PersonSearch{Born: d}.query()
			Expect(err).To(HaveOccurred(), "%+v", d)
		}
	})
}

// TestSearchPersonsDecodesARealPage checks both record kinds decode from a real
// answer: the regular person's owner, place and structured date, and the
// catalog record's catalog.
func TestSearchPersonsDecodesARealPage(t *testing.T) {
	RegisterTestingT(t)
	var got *http.Request
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = io.WriteString(w, wireSearchPage)
	})
	defer srv.Close()

	page, err := newTestClient(srv).SearchPersons(context.Background(), PersonSearch{LastName: "Мальчиков"})

	Expect(err).ToNot(HaveOccurred())
	Expect(got.URL.Path).To(Equal("/api/v3/persons"))
	Expect(got.URL.Query().Get("lastName")).To(Equal("Мальчиков"))
	Expect(got.Header.Get("Authorization")).To(Equal("Bearer " + testJWT()))
	Expect(page.Pager.TotalItems).To(Equal(1253))
	Expect(page.Persons).To(HaveLen(2))

	regular := page.Persons[0]
	Expect(regular.Type).To(Equal("regularPerson"))
	Expect(regular.UUID).To(Equal("982b57c4-970b-4d26-a7dc-ee6ab07c0aae"))
	Expect(regular.DisplayName).To(Equal("Мальчишков Мальчиков Василий Алексеевич"))
	Expect(regular.OwnerID).To(Equal("9812dabd-4acf-4891-9fc1-b9a44a203cd2"))
	Expect(regular.Gender).To(Equal(GenderMale))
	Expect(regular.PrivacyType).To(Equal("visible_for_all"))
	Expect(regular.BirthPlace).ToNot(BeNil())
	Expect(regular.BirthPlace.PrimaryName).To(Equal("Леднево"))
	Expect(regular.BirthPlace.MainGeorequisite.Level1).To(Equal("Ленинградская область"))
	Expect(regular.DeathPlace).To(BeNil())
	Expect(regular.BirthDate).ToNot(BeNil())
	Expect(regular.BirthDate.First.Year).To(Equal(1892))
	Expect(RangeFromEventDate(*regular.BirthDate).Year).To(Equal(1892))
	Expect(regular.DeathDate.First).To(BeNil(), "an unknown date has no parts")
	Expect(regular.HasDeathEvent).To(BeTrue())

	catalog := page.Persons[1]
	Expect(catalog.Type).To(Equal("catalogPerson"))
	Expect(catalog.CatalogKey).To(Equal("gwarmil"))
	Expect(catalog.CatalogName).To(HavePrefix("Списки участников Первой Мировой войны"))
	Expect(catalog.BirthDate).To(BeNil())
	Expect(catalog.OwnerID).To(BeEmpty())
}

// TestSearchPersonsWithoutSessionIsAnonymous covers the public search: no `t`
// cookie, no bearer, and no token scrape.
func TestSearchPersonsWithoutSessionIsAnonymous(t *testing.T) {
	RegisterTestingT(t)
	var scraped atomic.Bool
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			scraped.Store(true)
			return
		}
		auth.Store(r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, wireSearchPage)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	page, err := client.SearchPersons(context.Background(), PersonSearch{LastName: "Мальчиков"})

	Expect(err).ToNot(HaveOccurred())
	Expect(page.Persons).To(HaveLen(2))
	Expect(auth.Load()).To(Equal(""))
	Expect(scraped.Load()).To(BeFalse())
}

// TestSearchPersonsMineNeedsASession checks the own-persons filter fails up
// front without a session, instead of familio's 409 «Фильтр "мои персоны"
// недоступен без авторизации», which would read as a version conflict.
func TestSearchPersonsMineNeedsASession(t *testing.T) {
	RegisterTestingT(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, wireSearchPage)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	_, err := client.SearchPersons(context.Background(),
		PersonSearch{LastName: "Мальчиков", Types: []string{PersonSearchMine}})

	Expect(err).To(MatchError(ErrNotLoggedIn))
	Expect(hits.Load()).To(BeZero())
}
