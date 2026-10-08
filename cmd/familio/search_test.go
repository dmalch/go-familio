package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"

	familio "github.com/dmalch/go-familio"
	. "github.com/onsi/gomega"
)

func intPtr(v int) *int { return &v }

// TestPersonSearchFlags_Search checks every flag lands on the library query.
func TestPersonSearchFlags_Search(t *testing.T) {
	g := NewWithT(t)
	fs := (&globalOpts{stderr: &bytes.Buffer{}}).newFlagSet("person search")
	var p personSearchFlags
	p.register(fs)
	_, err := parseFlags(fs, []string{
		"-last", "Мальчиков", "-last-exact",
		"-first", "Иван", "-first-exact",
		"-text", "Мальчиков Иван",
		"-type", "catalog", "-type", "others",
		"-gender", "female",
		"-born", "1850..1860", "-died", "..1942",
		"-order", "born", "-asc",
		"-page", "2", "-limit", "50",
	})
	g.Expect(err).ToNot(HaveOccurred())

	s, err := p.search()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(s).To(Equal(familio.PersonSearch{
		LastName:                "Мальчиков",
		LastNameExact:           true,
		FirstAndMiddleName:      "Иван",
		FirstAndMiddleNameExact: true,
		Text:                    "Мальчиков Иван",
		Types:                   []string{familio.PersonSearchCatalog, familio.PersonSearchOtherUsers},
		Gender:                  familio.GenderFemale,
		Born:                    &familio.DateRange{Year: 1850, Range: familio.RangeBetween, EndYear: intPtr(1860)},
		Died:                    &familio.DateRange{Year: 1942, Range: familio.RangeBefore},
		OrderBy:                 familio.SearchOrderBirth,
		Ascending:               true,
		Page:                    2,
		ItemsPerPage:            50,
	}))
}

// TestParseDateSpan covers the -born/-died forms: one date, a closed range,
// and a range open on either side.
func TestParseDateSpan(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want *familio.DateRange
	}{
		{"1892", &familio.DateRange{Year: 1892}},
		{"1892-05-03", &familio.DateRange{Year: 1892, Month: intPtr(5), Day: intPtr(3)}},
		{"1850..1860", &familio.DateRange{Year: 1850, Range: familio.RangeBetween, EndYear: intPtr(1860)}},
		{"1900-02..1900-03", &familio.DateRange{
			Year: 1900, Month: intPtr(2), Range: familio.RangeBetween, EndYear: intPtr(1900), EndMonth: intPtr(3),
		}},
		{"1950..", &familio.DateRange{Year: 1950, Range: familio.RangeAfter}},
		{"..1800", &familio.DateRange{Year: 1800, Range: familio.RangeBefore}},
	} {
		t.Run(tc.in, func(t *testing.T) {
			g := NewWithT(t)
			got, err := parseDateSpan(tc.in)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tc.want))
		})
	}

	for _, bad := range []string{"..", "abc", "1850..abc", "1850..1860..1870", "1892-05-03-01"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			g := NewWithT(t)
			_, err := parseDateSpan(bad)
			g.Expect(err).To(HaveOccurred())
		})
	}
}

// TestPersonSearchFlags_Validation checks bad input is refused before any
// request, with a message naming the accepted values.
func TestPersonSearchFlags_Validation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no name criterion", []string{"-gender", "male"}, "-last, -first or -text"},
		{"unknown type", []string{"-last", "X", "-type", "friends"}, "mine|catalog|others"},
		{"unknown order", []string{"-last", "X", "-order", "age"}, "score|name|place|updated|born|died"},
		{"unknown gender", []string{"-last", "X", "-gender", "m"}, "male|female"},
		{"bad date", []string{"-last", "X", "-born", "18xx"}, "-born"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			fs := (&globalOpts{stderr: &bytes.Buffer{}}).newFlagSet("person search")
			var p personSearchFlags
			p.register(fs)
			_, err := parseFlags(fs, tc.args)
			g.Expect(err).ToNot(HaveOccurred())
			_, err = p.search()
			g.Expect(err).To(MatchError(ContainSubstring(tc.want)))
		})
	}
}

// TestRunPersonSearch_QueriesV3 runs the command end to end: it asks
// /api/v3/persons with the filters and prints both record kinds.
func TestRunPersonSearch_QueriesV3(t *testing.T) {
	g := NewWithT(t)
	var got *http.Request
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = io.WriteString(w, `{"pager":{"page":1,"itemsPerPage":20,"totalItems":2},"data":[
			{"uuid":"p-1","type":"regularPerson","displayName":"Мальчиков Иван Игнатьевич","ownerId":"o-1",
			 "birthDate":{"type":"equal","calendar":"gregorian","first":{"year":1855,"month":null,"day":null,"type":"gregorian"},"second":null,"formatted":"1855"}},
			{"uuid":"c-1","type":"catalogPerson","displayName":"Мальчиков Михаил","catalogKey":"gwarmil","birthDate":null}]}`)
	})

	code, out, errb := runArgs("-cookies", cookies, "person", "search", "-last", "Мальчиков",
		"-type", "others", "-born", "1850..1860")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(got.URL.Path).To(Equal("/api/v3/persons"))
	g.Expect(got.Header.Get("Authorization")).To(Equal("Bearer " + testJWT()))
	q := got.URL.Query()
	g.Expect(q.Get("lastName")).To(Equal("Мальчиков"))
	g.Expect(q.Get("types[0]")).To(Equal("other_users_persons"))
	g.Expect(q.Get("birthDate[from][year]")).To(Equal("1850"))
	g.Expect(q.Get("birthDate[till][day]")).To(Equal("31"))
	g.Expect(out).To(ContainSubstring(`"displayName": "Мальчиков Иван Игнатьевич"`))
	g.Expect(out).To(ContainSubstring(`"catalogKey": "gwarmil"`))
	g.Expect(out).To(ContainSubstring(`"totalItems": 2`))
}

// TestRunPersonSearch_PublicWithoutCredentials checks the search runs with no
// credentials at all, sending no bearer.
func TestRunPersonSearch_PublicWithoutCredentials(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("FAMILIO_COOKIES", "")
	t.Setenv("FAMILIO_SESSION", "")
	t.Setenv("FAMILIO_BROWSER", "")
	var query url.Values
	var auth string
	serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		query, auth = r.URL.Query(), r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"pager":{"page":1,"itemsPerPage":20,"totalItems":0},"data":[]}`)
	})

	code, _, errb := runArgs("person", "search", "-text", "Мальчиков Иван")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(query.Get("name")).To(Equal("Мальчиков Иван"))
	g.Expect(auth).To(BeEmpty())
}

// TestRun_Help_ListsPersonSearch keeps the command discoverable.
func TestRun_Help_ListsPersonSearch(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	g.Expect(out).To(MatchRegexp(`(?m)^  person search\s+`))
}
