package main

import (
	"io"
	"net/http"
	"testing"

	familio "github.com/dmalch/go-familio"
	. "github.com/onsi/gomega"
)

// TestHTMLToText covers the markup catalog values carry: line breaks become
// newlines, tags (emphasis, links) are dropped, entities are unescaped.
func TestHTMLToText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Урожд. - <b>Иванов Фома Иванов</b><br>Отец - <b>Иванов Иван</b>", "Урожд. - Иванов Фома Иванов\nОтец - Иванов Иван"},
		{`<a  href="https://familio.org/settlements/f4f7">ст. Щучинск, Кокчетавский у.</a>`, "ст. Щучинск, Кокчетавский у."},
		{"раз<br/>два<BR />три", "раз\nдва\nтри"},
		{"Резервный&nbsp;Казак &amp; сын", "Резервный Казак & сын"},
		{"  plain text  ", "plain text"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(htmlToText(tc.in)).To(Equal(tc.want))
		})
	}
}

// TestBuildCatalogPersonView checks the readable view: the record's fields in
// the catalog's order under their titles, only those the card shows and that
// have a value, plus the dates, places and links.
func TestBuildCatalogPersonView(t *testing.T) {
	g := NewWithT(t)
	p := &familio.CatalogPerson{
		UUID:       "f538a502-0cd1-4cf4-bac7-a77f55d90c92",
		CatalogKey: "mkkoturkul",
		Text:       "Иванов Фома Иванов",
		BirthDate:  &familio.EventDate{Formatted: "06.07.1859 ст."},
		Geography:  []string{"ст. Щучинск"},
		Record: map[string]any{
			"uuid":         "row-1",
			"position":     float64(1483),
			"record_text":  "Иванов Фома Иванов",
			"baptism_date": "07.07.1859",
			"year":         "1859",
			"priest":       "",
			"full_record":  "Урожд. - <b>Иванов</b><br>Отец - <b>Иван</b>",
			"record_n":     float64(297),
		},
		LinkedPersons: []familio.CatalogLinkedPerson{{UUID: "p-1", LastName: "Иванов", FirstName: "Фома"}},
		Settlements:   []familio.CatalogSettlement{{UUID: "s-1", Name: "Щучинск", Type: "город"}},
	}
	cat := &familio.Catalog{
		Key: "mkkoturkul", Name: "Метрические книги", Years: "1854-1876",
		Fields: []familio.CatalogField{
			{Key: "record_text", Title: "ФИО", ShowInCard: true},
			{Key: "record_n", Title: "Номер в справочнике", ShowInCard: true},
			{Key: "year", Title: "Год", ShowInCard: false},
			{Key: "baptism_date", Title: "Дата крещения", ShowInCard: true},
			{Key: "priest", Title: "Священнослужители", ShowInCard: true},
			{Key: "full_record", Title: "Полная запись МК", ShowInCard: true},
		},
	}

	v := buildCatalogPersonView(p, cat, "https://familio.org/")

	g.Expect(v.URL).To(Equal("https://familio.org/catalogs/mkkoturkul/persons/f538a502-0cd1-4cf4-bac7-a77f55d90c92"))
	g.Expect(v.Catalog).To(Equal(catalogRef{Key: "mkkoturkul", Name: "Метрические книги", Years: "1854-1876"}))
	g.Expect(v.BirthDate).To(Equal("06.07.1859 ст."))
	g.Expect(v.DeathDate).To(BeEmpty())
	g.Expect(v.Fields).To(Equal([]catalogFieldView{
		{Key: "record_text", Title: "ФИО", Value: "Иванов Фома Иванов"},
		{Key: "record_n", Title: "Номер в справочнике", Value: "297"},
		{Key: "baptism_date", Title: "Дата крещения", Value: "07.07.1859"},
		{Key: "full_record", Title: "Полная запись МК", Value: "Урожд. - Иванов\nОтец - Иван"},
	}), "card fields with a value, in catalog order; year is table-only, priest is empty")
	g.Expect(v.LinkedPersons).To(Equal([]linkView{{UUID: "p-1", Name: "Иванов Фома"}}))
	g.Expect(v.Settlements).To(Equal([]linkView{{UUID: "s-1", Name: "Щучинск (город)"}}))
}

// TestParseCatalogPersonArgs covers both ways of naming a record: its catalog
// key and uuid, or the site's link to it.
func TestParseCatalogPersonArgs(t *testing.T) {
	const id = "774b6dcb-b44a-4304-944b-d7a5d4513c61"
	for _, args := range [][]string{
		{"gwarmil", id},
		{"https://familio.org/catalogs/gwarmil/persons/" + id},
		{"https://familio.org/catalogs/gwarmil/persons/" + id + "?tab=1"},
		{"/catalogs/gwarmil/persons/" + id},
	} {
		g := NewWithT(t)
		key, uuid, err := parseCatalogPersonArgs(args)
		g.Expect(err).ToNot(HaveOccurred(), "%v", args)
		g.Expect(key).To(Equal("gwarmil"))
		g.Expect(uuid).To(Equal(id))
	}
	for _, args := range [][]string{
		nil,
		{"gwarmil"},
		{"https://familio.org/persons/" + id},
		{"gwarmil", id, "extra"},
	} {
		g := NewWithT(t)
		_, _, err := parseCatalogPersonArgs(args)
		g.Expect(err).To(MatchError(ContainSubstring("<catalog-key> <uuid>")), "%v", args)
	}
}

// TestRunCatalogPerson_PrintsTheLabelledRecord runs the command end to end: it
// reads the record and its catalog from /api/v1 and prints the labelled view.
func TestRunCatalogPerson_PrintsTheLabelledRecord(t *testing.T) {
	g := NewWithT(t)
	var paths []string
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/catalogs/gwarmil/excerpts/774b6dcb-b44a-4304-944b-d7a5d4513c61":
			_, _ = io.WriteString(w, `{"uuid":"774b6dcb-b44a-4304-944b-d7a5d4513c61","recordID":"r-1","excerptsType":"person",
				"excerptsText":"Мальчиков Михаил ","catalogKey":"gwarmil",
				"attributes":{"geography":["Владимирская губерния, Юрьевский уезд"]},
				"record":{"uuid":"r-1","position":38277540,"record_text":"Мальчиков Михаил ",
				  "url":"<a href=\"https://gwar.mil.ru/heroes/chelovek_donesenie11908382/\" target=\"_blank\">https://gwar.mil.ru/heroes/chelovek_donesenie11908382/</a>"},
				"persons":[],"parishes":[],"settlements":[],"birth_date":null,"death_date":null}`)
		case "/api/v1/catalogs/gwarmil":
			_, _ = io.WriteString(w, `{"uuid":"c-1","key":"gwarmil","name":"Списки участников Первой Мировой войны",
				"issueYearDescription":"1914-1918","catalogsFields":[
				  {"key":"record_text","title":"ФИО","type":"string","showInCard":true},
				  {"key":"url","title":"Ссылка на источник","type":"string","showInCard":true}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	})

	code, out, errb := runArgs("-cookies", cookies, "catalog", "person",
		"https://familio.org/catalogs/gwarmil/persons/774b6dcb-b44a-4304-944b-d7a5d4513c61")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(paths).To(ConsistOf(
		"/api/v1/catalogs/gwarmil/excerpts/774b6dcb-b44a-4304-944b-d7a5d4513c61", "/api/v1/catalogs/gwarmil"))
	g.Expect(out).To(ContainSubstring(`"name": "Списки участников Первой Мировой войны"`))
	g.Expect(out).To(ContainSubstring(`"title": "Ссылка на источник"`))
	g.Expect(out).To(ContainSubstring(`"value": "https://gwar.mil.ru/heroes/chelovek_donesenie11908382/"`))
	g.Expect(out).To(ContainSubstring(`"Владимирская губерния, Юрьевский уезд"`))
	g.Expect(out).ToNot(ContainSubstring("<a href"), "values are shown as text")
}

// TestRunCatalogGet_PrintsTheCatalog covers the catalog read.
func TestRunCatalogGet_PrintsTheCatalog(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.Path).To(Equal("/api/v1/catalogs/gwarmil"))
		_, _ = io.WriteString(w, `{"uuid":"c-1","key":"gwarmil","name":"Списки участников","recordsCount":6489681,
			"catalogsFields":[{"key":"record_text","title":"ФИО","showInCard":true}]}`)
	})

	code, out, errb := runArgs("-cookies", cookies, "catalog", "get", "gwarmil")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(out).To(ContainSubstring(`"recordsCount": 6489681`))
	g.Expect(out).To(ContainSubstring(`"title": "ФИО"`))
}

// TestRunCatalogPerson_RejectsBadArgs checks a malformed call fails before any
// request.
func TestRunCatalogPerson_RejectsBadArgs(t *testing.T) {
	g := NewWithT(t)
	serveAPI(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	})

	code, _, errb := runArgs("catalog", "person", "gwarmil")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("<catalog-key> <uuid>"))

	code, _, errb = runArgs("catalog", "person", "gwarmil", "not-a-uuid")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("invalid request"))
}

// TestRun_Help_ListsCatalogCommands keeps the commands discoverable.
func TestRun_Help_ListsCatalogCommands(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	g.Expect(out).To(MatchRegexp(`(?m)^  catalog get\s+`))
	g.Expect(out).To(MatchRegexp(`(?m)^  catalog person\s+`))
}
