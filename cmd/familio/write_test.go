package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// runStdin is runArgs with a stdin body, for the commands that read text from it.
func runStdin(stdin string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestTwoArgs(t *testing.T) {
	g := NewWithT(t)

	a, b, err := twoArgs([]string{"u1", "u2"}, "person-uuid", "union-uuid")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(a).To(Equal("u1"))
	g.Expect(b).To(Equal("u2"))

	for _, args := range [][]string{nil, {"u1"}, {"u1", "u2", "u3"}, {"", "u2"}, {"u1", ""}} {
		_, _, err = twoArgs(args, "person-uuid", "union-uuid")
		g.Expect(err).To(HaveOccurred(), "expected %v to be rejected", args)
		g.Expect(err.Error()).To(ContainSubstring("<person-uuid> <union-uuid>"))
	}
}

// TestRunMarriageCreate_PostsAWeddingEvent locks the request the command builds:
// a wedding event with both persons as spouse participants, anchored under the
// first partner, carrying the parsed date and the comment.
func TestRunMarriageCreate_PostsAWeddingEvent(t *testing.T) {
	g := NewWithT(t)
	var gotPath, gotContentType string
	var sent map[string]any

	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		g.Expect(json.Unmarshal(body, &sent)).To(Succeed())
		_, _ = io.WriteString(w, `{"uuid":"union-1","type":"wedding",
			"date":{"calendar":"gregorian","type":"equal","first":{"year":1914,"month":10,"day":5},
			        "formatted":"5 октября 1914"},
			"participants":[{"personUuid":"pa","role":"spouse"},{"personUuid":"pb","role":"spouse"}],
			"comment":"МК Журавкино"}`)
	})

	code, out, errb := runStdin("", "-cookies", cookies, "marriage", "create", "pa", "pb",
		"-date", "1914-10-05", "-comment", "МК Журавкино")
	g.Expect(code).To(Equal(0), errb)

	g.Expect(gotPath).To(Equal("/api/v2/persons/pa/events"), "the event is anchored under the first partner")
	g.Expect(gotContentType).To(Equal("application/ld+json"))
	g.Expect(sent["type"]).To(Equal("wedding"))
	g.Expect(sent["comment"]).To(Equal("МК Журавкино"))

	participants := asSlice(t, sent["participants"])
	g.Expect(participants).To(HaveLen(2))
	for _, p := range participants {
		g.Expect(asMap(t, p)["role"]).To(Equal("spouse"))
	}
	date := asMap(t, asMap(t, sent["date"])["first"])
	g.Expect(date["year"]).To(BeNumerically("==", 1914))
	g.Expect(date["month"]).To(BeNumerically("==", 10))
	g.Expect(date["day"]).To(BeNumerically("==", 5))

	g.Expect(out).To(ContainSubstring(`"uuid": "union-1"`))
}

// TestRunMarriageCreate_OmittedDateSendsNoDate covers the optional-date path: an
// unknown wedding date must still create the link.
func TestRunMarriageCreate_OmittedDateSendsNoDate(t *testing.T) {
	g := NewWithT(t)
	var sent map[string]any
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		g.Expect(json.Unmarshal(body, &sent)).To(Succeed())
		_, _ = io.WriteString(w, `{"uuid":"union-2","type":"wedding"}`)
	})

	code, _, errb := runStdin("", "-cookies", cookies, "marriage", "create", "pa", "pb")
	g.Expect(code).To(Equal(0), errb)

	g.Expect(asMap(t, sent["date"])["first"]).To(BeNil(), "an omitted -date must not invent a year")
}

func TestRunMarriageCreate_InvalidDateIsRejectedBeforeAnyRequest(t *testing.T) {
	g := NewWithT(t)
	var requests int
	cookies := serveAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	})

	code, _, errb := runStdin("", "-cookies", cookies, "marriage", "create", "pa", "pb", "-date", "not-a-year")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("invalid year"))
	g.Expect(requests).To(BeZero(), "a bad date must not reach the API")
}

// TestRunMarriageDelete_DeletesUnderTheParticipant covers the delete path and its
// confirmation output.
func TestRunMarriageDelete_DeletesUnderTheParticipant(t *testing.T) {
	g := NewWithT(t)
	var gotMethod, gotPath string
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	code, out, errb := runStdin("", "-cookies", cookies, "marriage", "delete", "pa", "union-1")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(gotMethod).To(Equal(http.MethodDelete))
	g.Expect(gotPath).To(Equal("/api/v2/persons/pa/events/union-1"))
	g.Expect(out).To(ContainSubstring(`"deleted": "union-1"`))
}

func TestRunMarriageDelete_MissingArgIsCommandError(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runStdin("", "marriage", "delete", "pa")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("<person-uuid> <union-uuid>"))
}

// TestRunPersonSetBiography_ReplacesUsingTheBiographyVersion locks the two-step
// write and — the subtle part — that the optimistic-lock token is the
// biography's own updatedAt, not the person's.
func TestRunPersonSetBiography_ReplacesUsingTheBiographyVersion(t *testing.T) {
	g := NewWithT(t)
	var gotVersion, gotMethod string
	var sent map[string]any

	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.Path).To(Equal("/api/v2/persons/p1/biography"))
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"text":"старый текст","updatedAt":"2026-07-14T21:37:12+00:00"}`)
			return
		}
		gotMethod = r.Method
		gotVersion = r.Header.Get("X-Base-Version")
		body, _ := io.ReadAll(r.Body)
		g.Expect(json.Unmarshal(body, &sent)).To(Succeed())
		_, _ = io.WriteString(w, `{"text":"новый текст","updatedAt":"2026-07-30T10:00:00+00:00"}`)
	})

	code, out, errb := runStdin("", "-cookies", cookies, "person", "set-biography", "p1", "-text", "новый текст")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(gotMethod).To(Equal(http.MethodPut))
	g.Expect(gotVersion).To(Equal("2026-07-14T21:37:12+00:00"))
	g.Expect(sent["text"]).To(Equal("новый текст"))
	g.Expect(out).To(ContainSubstring("новый текст"))
}

// TestRunPersonSetBiography_AppendKeepsTheExistingText covers -append, which must
// preserve what is there and separate the addition with a blank line.
func TestRunPersonSetBiography_AppendKeepsTheExistingText(t *testing.T) {
	g := NewWithT(t)
	var sent map[string]any
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"text":"первая строка","updatedAt":"v1"}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		g.Expect(json.Unmarshal(body, &sent)).To(Succeed())
		_, _ = io.WriteString(w, `{"text":"ok","updatedAt":"v2"}`)
	})

	code, _, errb := runStdin("", "-cookies", cookies, "person", "set-biography", "p1",
		"-append", "-text", "вторая строка")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(sent["text"]).To(Equal("первая строка\n\nвторая строка"))
}

// TestRunPersonSetBiography_ReadsStdinWhenTextOmitted covers the piped-input path.
func TestRunPersonSetBiography_ReadsStdinWhenTextOmitted(t *testing.T) {
	g := NewWithT(t)
	var sent map[string]any
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"text":"","updatedAt":"v1"}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		g.Expect(json.Unmarshal(body, &sent)).To(Succeed())
		_, _ = io.WriteString(w, `{"text":"ok","updatedAt":"v2"}`)
	})

	code, _, errb := runStdin("текст из stdin", "-cookies", cookies, "person", "set-biography", "p1")
	g.Expect(code).To(Equal(0), errb)
	g.Expect(sent["text"]).To(Equal("текст из stdin"))
}

// TestRunPersonSetBiography_StaleVersionIsReported proves the 409 reaches the user
// as a conflict they can act on rather than an opaque HTTP error.
func TestRunPersonSetBiography_StaleVersionIsReported(t *testing.T) {
	g := NewWithT(t)
	cookies := serveAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"text":"","updatedAt":"stale"}`)
			return
		}
		w.WriteHeader(http.StatusConflict)
	})

	code, _, errb := runStdin("", "-cookies", cookies, "person", "set-biography", "p1", "-text", "новый")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("HTTP 409"))
	g.Expect(errb).To(ContainSubstring("stale X-Base-Version"))
}
