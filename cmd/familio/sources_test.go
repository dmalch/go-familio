package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
)

const (
	testPerson = "11111111-2222-3333-4444-555555555555"
	testRecord = "774b6dcb-b44a-4304-944b-d7a5d4513c61"
)

// sourcesFake is a fake familio.org for the source commands: one person, one
// catalog record, and the person's source list, which the writes change.
type sourcesFake struct {
	mu       sync.Mutex
	sources  []map[string]any
	requests []string // "METHOD path"
	posted   map[string]any
	patched  map[string]any
	patchVer string
}

func newSourcesFake(t *testing.T, existing ...map[string]any) (*sourcesFake, string) {
	t.Helper()
	f := &sourcesFake{sources: existing}
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		sources := "/api/v2/persons/" + testPerson + "/sources"
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/catalogs/gwarmil/excerpts/"+testRecord:
			_, _ = io.WriteString(w, `{"uuid":"`+testRecord+`","recordID":"r-1","excerptsText":"Мальчиков Михаил ",
				"catalogKey":"gwarmil","attributes":[],"record":{"record_text":"Мальчиков Михаил "},
				"persons":[],"settlements":[],"birth_date":null,"death_date":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/persons/"+testPerson:
			_, _ = io.WriteString(w, `{"uuid":"`+testPerson+`","displayName":"Источникова АкцТест"}`)
		case r.Method == http.MethodGet && r.URL.Path == sources:
			_ = json.NewEncoder(w).Encode(f.sources)
		case r.Method == http.MethodPost && r.URL.Path == sources:
			_ = json.NewDecoder(r.Body).Decode(&f.posted)
			created := map[string]any{"uuid": testRecord, "type": "catalog_person", "comment": "",
				"name": "Списки участников Первой Мировой войны", "requisites": "Мальчиков Михаил ",
				"catalog":   map[string]any{"key": "gwarmil", "hidden": false},
				"updatedAt": "2026-10-08T15:30:00+00:00"}
			f.sources = append(f.sources, created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodPatch && r.URL.Path == sources+"/"+testRecord:
			_ = json.NewDecoder(r.Body).Decode(&f.patched)
			f.patchVer = r.Header.Get("X-Base-Version")
			f.sources[len(f.sources)-1]["comment"] = f.patched["comment"]
			_ = json.NewEncoder(w).Encode(f.sources[len(f.sources)-1])
		case r.Method == http.MethodDelete && r.URL.Path == sources+"/"+testRecord:
			f.sources = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	})
	return f, cookies
}

func (f *sourcesFake) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *sourcesFake) writes() []string {
	var out []string
	for _, r := range f.seen() {
		if !strings.HasPrefix(r, http.MethodGet+" ") {
			out = append(out, r)
		}
	}
	return out
}

// TestRunSourcesAdd_CitesTheRecordWithAComment runs the whole add: read the
// record, the person and their sources, create the catalog_person source, then
// set the comment — the provider's own create path.
func TestRunSourcesAdd_CitesTheRecordWithAComment(t *testing.T) {
	g := NewWithT(t)
	f, cookies := newSourcesFake(t)
	sources := "/api/v2/persons/" + testPerson + "/sources"

	code, out, errb := runArgs("-cookies", cookies, "sources", "add", testPerson, "gwarmil", testRecord,
		"-comment", "проверка CLI", "-yes")

	g.Expect(code).To(Equal(0), errb)
	g.Expect(f.seen()).To(Equal([]string{
		"GET /api/v1/catalogs/gwarmil/excerpts/" + testRecord,
		"GET /api/v2/persons/" + testPerson,
		"GET " + sources,
		"POST " + sources,
		"GET " + sources,
		"PATCH " + sources + "/" + testRecord,
	}))
	g.Expect(f.posted).To(Equal(map[string]any{"uuid": testRecord, "type": "catalog_person", "catalogKey": "gwarmil"}))
	g.Expect(f.patched).To(Equal(map[string]any{"comment": "проверка CLI"}))
	g.Expect(f.patchVer).To(Equal("2026-10-08T15:30:00+00:00"), "the comment edit carries the optimistic lock")
	g.Expect(out).To(ContainSubstring(`"comment": "проверка CLI"`))
	g.Expect(out).To(ContainSubstring(`"type": "catalog_person"`))
}

// TestRunSourcesAdd_TakesTheRecordLink covers naming the record by its familio
// link, and that no comment means no comment edit.
func TestRunSourcesAdd_TakesTheRecordLink(t *testing.T) {
	g := NewWithT(t)
	f, cookies := newSourcesFake(t)

	code, _, errb := runArgs("-cookies", cookies, "sources", "add", "-yes", testPerson,
		"https://familio.org/catalogs/gwarmil/persons/"+testRecord)

	g.Expect(code).To(Equal(0), errb)
	g.Expect(f.writes()).To(Equal([]string{"POST /api/v2/persons/" + testPerson + "/sources"}))
}

// TestRunSourcesAdd_DeclinedPromptWritesNothing checks the confirmation: it names
// the record and the person, and anything but yes — an empty stdin included —
// aborts before the write.
func TestRunSourcesAdd_DeclinedPromptWritesNothing(t *testing.T) {
	for _, stdin := range []string{"n\n", ""} {
		t.Run(fmt.Sprintf("stdin %q", stdin), func(t *testing.T) {
			g := NewWithT(t)
			f, cookies := newSourcesFake(t)

			code, _, errb := runStdin(stdin, "-cookies", cookies, "sources", "add", testPerson, "gwarmil", testRecord)

			g.Expect(code).To(Equal(1))
			g.Expect(errb).To(ContainSubstring("Мальчиков Михаил"))
			g.Expect(errb).To(ContainSubstring("Источникова АкцТест"))
			g.Expect(errb).To(ContainSubstring("add aborted"))
			g.Expect(f.writes()).To(BeEmpty())
		})
	}
}

// TestRunSourcesAdd_ConfirmedPromptWrites checks a yes goes ahead.
func TestRunSourcesAdd_ConfirmedPromptWrites(t *testing.T) {
	g := NewWithT(t)
	f, cookies := newSourcesFake(t)

	code, _, errb := runStdin("y\n", "-cookies", cookies, "sources", "add", testPerson, "gwarmil", testRecord)

	g.Expect(code).To(Equal(0), errb)
	g.Expect(f.writes()).To(HaveLen(1))
}

// TestRunSourcesAdd_RefusesADuplicate checks a record the person already cites
// is refused before any write: familio keys a source by the cited entity.
func TestRunSourcesAdd_RefusesADuplicate(t *testing.T) {
	g := NewWithT(t)
	f, cookies := newSourcesFake(t, map[string]any{"uuid": testRecord, "type": "catalog_person"})

	code, _, errb := runArgs("-cookies", cookies, "sources", "add", "-yes", testPerson, "gwarmil", testRecord)

	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("already cites"))
	g.Expect(f.writes()).To(BeEmpty())
}

// TestRunSourcesAdd_RejectsBadArgs checks malformed calls fail before any write.
func TestRunSourcesAdd_RejectsBadArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no record", []string{testPerson}, "<person-uuid> <catalog-key> <record-uuid>"},
		{"no args", nil, "<person-uuid> <catalog-key> <record-uuid>"},
		{"extra argument", []string{testPerson, "gwarmil", testRecord, "more"}, "<person-uuid> <catalog-key> <record-uuid>"},
		{"malformed record id", []string{testPerson, "gwarmil", "not-a-uuid"}, "invalid request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			f, cookies := newSourcesFake(t)

			code, _, errb := runArgs(append([]string{"-cookies", cookies, "sources", "add", "-yes"}, tc.args...)...)

			g.Expect(code).To(Equal(1))
			g.Expect(errb).To(ContainSubstring(tc.want))
			g.Expect(f.writes()).To(BeEmpty())
		})
	}
}

// TestRunSourcesRemove covers the undo: the source the person cites is deleted
// by its record uuid or link, a source the person does not have is an error, and
// a declined prompt deletes nothing.
func TestRunSourcesRemove(t *testing.T) {
	cited := func() map[string]any {
		// The shape familio returns: the catalog's name in name, the record's text in requisites.
		return map[string]any{"uuid": testRecord, "type": "catalog_person",
			"name": "Списки участников Первой Мировой войны", "requisites": "Мальчиков Михаил "}
	}
	t.Run("by uuid", func(t *testing.T) {
		g := NewWithT(t)
		f, cookies := newSourcesFake(t, cited())

		code, out, errb := runArgs("-cookies", cookies, "sources", "remove", testPerson, testRecord, "-yes")

		g.Expect(code).To(Equal(0), errb)
		g.Expect(f.writes()).To(Equal([]string{"DELETE /api/v2/persons/" + testPerson + "/sources/" + testRecord}))
		g.Expect(out).To(ContainSubstring(`"removed": "` + testRecord + `"`))
	})
	t.Run("by link", func(t *testing.T) {
		g := NewWithT(t)
		f, cookies := newSourcesFake(t, cited())

		code, _, errb := runArgs("-cookies", cookies, "sources", "remove", "-yes", testPerson,
			"https://familio.org/catalogs/gwarmil/persons/"+testRecord)

		g.Expect(code).To(Equal(0), errb)
		g.Expect(f.writes()).To(HaveLen(1))
	})
	t.Run("not cited", func(t *testing.T) {
		g := NewWithT(t)
		f, cookies := newSourcesFake(t)

		code, _, errb := runArgs("-cookies", cookies, "sources", "remove", "-yes", testPerson, testRecord)

		g.Expect(code).To(Equal(1))
		g.Expect(errb).To(ContainSubstring("does not cite"))
		g.Expect(f.writes()).To(BeEmpty())
	})
	t.Run("declined", func(t *testing.T) {
		g := NewWithT(t)
		f, cookies := newSourcesFake(t, cited())

		code, _, errb := runStdin("n\n", "-cookies", cookies, "sources", "remove", testPerson, testRecord)

		g.Expect(code).To(Equal(1))
		g.Expect(errb).To(ContainSubstring("«Мальчиков Михаил» (Списки участников Первой Мировой войны)"))
		g.Expect(errb).To(ContainSubstring("remove aborted"))
		g.Expect(f.writes()).To(BeEmpty())
	})
}

// TestRun_Help_ListsSourceWrites keeps the commands discoverable.
func TestRun_Help_ListsSourceWrites(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	g.Expect(out).To(MatchRegexp(`(?m)^  sources add\s+`))
	g.Expect(out).To(MatchRegexp(`(?m)^  sources remove\s+`))
}
