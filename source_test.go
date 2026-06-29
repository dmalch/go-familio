package familio

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
)

// TestSourceRefWriteShape locks the create body familio accepts:
// {uuid, type, catalogKey}, with catalogKey EXPLICITLY null for a `case` (not
// omitted) and the catalog id for a `catalog_person`.
func TestSourceRefWriteShape(t *testing.T) {
	RegisterTestingT(t)

	caseBody, err := json.Marshal(SourceRef{UUID: "58e68fa4", Type: SourceTypeCase, CatalogKey: nil})
	Expect(err).ToNot(HaveOccurred())
	Expect(string(caseBody)).To(Equal(`{"uuid":"58e68fa4","type":"case","catalogKey":null}`))

	key := "gwarmil"
	catBody, err := json.Marshal(SourceRef{UUID: "0123e5fb", Type: SourceTypeCatalogPerson, CatalogKey: &key})
	Expect(err).ToNot(HaveOccurred())
	Expect(string(catBody)).To(Equal(`{"uuid":"0123e5fb","type":"catalog_person","catalogKey":"gwarmil"}`))
}

// TestSourceReadBack decodes the enriched object familio returns and confirms
// the server-derived fields land where expected.
func TestSourceReadBack(t *testing.T) {
	RegisterTestingT(t)
	const body = `{"uuid":"58e68fa4","type":"case","comment":"проба",
		"name":"Ревизские сказки","requisites":"ГИА … ф. 145 оп. 1 д. 431",
		"years":"1811 - 1811","catalog":null,
		"createdAt":"2026-06-28T09:48:38+00:00","updatedAt":"2026-06-28T09:48:38+00:00"}`
	var s Source
	Expect(json.Unmarshal([]byte(body), &s)).To(Succeed())
	Expect(s.UUID).To(Equal("58e68fa4"))
	Expect(s.Type).To(Equal(SourceTypeCase))
	Expect(s.Comment).To(Equal("проба"))
	Expect(s.Name).To(Equal("Ревизские сказки"))
	Expect(s.Years).To(Equal("1811 - 1811"))
}

// TestSourceCommentPatchShape locks the in-place edit body: only the comment.
func TestSourceCommentPatchShape(t *testing.T) {
	RegisterTestingT(t)
	b, err := json.Marshal(sourceCommentPatch{Comment: "новый"})
	Expect(err).ToNot(HaveOccurred())
	Expect(string(b)).To(Equal(`{"comment":"новый"}`))
}

func TestFindSourceByID(t *testing.T) {
	RegisterTestingT(t)
	sources := []Source{{UUID: "a", Type: SourceTypeCase}, {UUID: "b", Type: SourceTypeCatalogPerson}}
	Expect(FindSourceByID(sources, "b").Type).To(Equal(SourceTypeCatalogPerson))
	Expect(FindSourceByID(sources, "missing")).To(BeNil())
}

// TestSourceCatalogUnmarshal locks the catalog field decode: the familio API
// returns `catalog` as an OBJECT {key, hidden} for catalog_person sources (a
// `case` omits it), and the decoder also tolerates a bare string / null.
func TestSourceCatalogUnmarshal(t *testing.T) {
	RegisterTestingT(t)
	cases := map[string]string{
		`{"uuid":"u","type":"catalog_person","catalog":{"key":"vss","hidden":false}}`: "vss",
		`{"uuid":"u","type":"catalog_person","catalog":"gwarmil"}`:                    "gwarmil",
		`{"uuid":"u","type":"case","catalog":null}`:                                   "",
		`{"uuid":"u","type":"case"}`:                                                  "",
	}
	for body, want := range cases {
		var s Source
		Expect(json.Unmarshal([]byte(body), &s)).To(Succeed(), body)
		Expect(s.Catalog.String()).To(Equal(want), body)
	}
}
