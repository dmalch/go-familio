package familio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// tagsListFixture is the shape GET /users/<uuid>/tags returns: a bare array of
// tags, each carrying the server-computed isFree flag (see API.md "Tags
// sub-resource").
const tagsListFixture = `[
  {"id": 2832, "tag": "Проверить в архиве",
   "color": "mint-mist", "description": "Нужен запрос в ЦГА", "isFree": true},
  {"id": 2833, "tag": "Раскулачены",
   "color": "rose-mist", "description": "", "isFree": false}
]`

// TestListTags locks the owner-addressed list read and the tag decode.
func TestListTags(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodGet))
		Expect(r.URL.Path).To(Equal("/api/v2/users/" + testOwnerUUID + "/tags"))
		Expect(r.Header.Get("Authorization")).To(HavePrefix("Bearer eyJ"))
		_, _ = io.WriteString(w, tagsListFixture)
	})
	defer srv.Close()

	tags, err := newTestClient(srv).ListTags(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(tags).To(HaveLen(2))

	Expect(tags[0].ID).To(Equal(2832))
	Expect(tags[0].Name).To(Equal("Проверить в архиве"))
	Expect(tags[0].Color).To(Equal(TagColorMintMist))
	Expect(tags[0].Description).To(Equal("Нужен запрос в ЦГА"))
	Expect(tags[0].IsFree).To(BeTrue())

	Expect(tags[1].Name).To(Equal("Раскулачены"))
	Expect(tags[1].Description).To(BeEmpty())
	Expect(tags[1].IsFree).To(BeFalse())
}

// TestTagInputEncoding pins the wire names of the writable fields: the name
// travels as "tag", and the embedded struct's fields are promoted rather than
// nested.
func TestTagInputEncoding(t *testing.T) {
	RegisterTestingT(t)

	encoded, err := json.Marshal(TagInput{Name: "Метка", Color: TagColorIceBlue, Description: "текст"})
	Expect(err).ToNot(HaveOccurred())

	var sent map[string]any
	Expect(json.Unmarshal(encoded, &sent)).To(Succeed())
	Expect(sent).To(HaveLen(3))
	Expect(sent["tag"]).To(Equal("Метка"))
	Expect(sent["color"]).To(Equal(TagColorIceBlue))
	Expect(sent["description"]).To(Equal("текст"))

	// A read-side Tag re-encodes flat too, so a decoded tag can be sent back.
	encoded, err = json.Marshal(Tag{
		TagInput: TagInput{Name: "Метка", Color: TagColorIceBlue},
		ID:       17, IsFree: true,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(json.Unmarshal(encoded, &sent)).To(Succeed())
	Expect(sent).To(HaveLen(5))
	Expect(sent["tag"]).To(Equal("Метка"))
	Expect(sent["id"]).To(BeEquivalentTo(17))
	Expect(sent["isFree"]).To(Equal(true))
}

// TestCreateTag locks the create path: POST /tags with the three writable
// fields, the name trimmed the way the editor trims it.
func TestCreateTag(t *testing.T) {
	RegisterTestingT(t)
	var gotBody []byte
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPost))
		Expect(r.URL.Path).To(Equal("/api/v2/tags"))
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		gotBody = body
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id": 2900, "tag": "Проверить", "color": "lemon-tint",
			"description": "", "isFree": true}`)
	})
	defer srv.Close()

	tag, err := newTestClient(srv).CreateTag(context.Background(),
		TagInput{Name: "  Проверить  ", Color: TagColorLemonTint})
	Expect(err).ToNot(HaveOccurred())

	var sent map[string]any
	Expect(json.Unmarshal(gotBody, &sent)).To(Succeed())
	Expect(sent["tag"]).To(Equal("Проверить"))
	Expect(sent["color"]).To(Equal(TagColorLemonTint))
	Expect(sent["description"]).To(BeEmpty())

	Expect(tag.ID).To(Equal(2900))
	Expect(tag.Name).To(Equal("Проверить"))
	Expect(tag.IsFree).To(BeTrue())
}

// TestUpdateTag locks the update path: a PUT addressed by tag id, with no
// optimistic-lock header (the tags endpoints carry none).
func TestUpdateTag(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPut))
		Expect(r.URL.Path).To(Equal("/api/v2/tags/7"))
		Expect(r.Header.Get("X-Base-Version")).To(BeEmpty())
		_, _ = io.WriteString(w, `{"id": 7, "tag": "Новое", "color": "lilac-glow",
			"description": "d", "isFree": false}`)
	})
	defer srv.Close()

	tag, err := newTestClient(srv).UpdateTag(context.Background(), 7,
		TagInput{Name: "Новое", Color: TagColorLilacGlow, Description: "d"})
	Expect(err).ToNot(HaveOccurred())
	Expect(tag.Name).To(Equal("Новое"))
	Expect(tag.Color).To(Equal(TagColorLilacGlow))
}

// TestDeleteTag locks the delete path.
func TestDeleteTag(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodDelete))
		Expect(r.URL.Path).To(Equal("/api/v2/tags/7"))
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	Expect(newTestClient(srv).DeleteTag(context.Background(), 7)).To(Succeed())
}

// TestTagInputValidate covers the editor's rules, which the client applies
// before it spends a request.
func TestTagInputValidate(t *testing.T) {
	RegisterTestingT(t)

	Expect(TagInput{Name: "ok", Color: TagColorRoseMist}.Validate()).To(Succeed())
	for _, color := range TagColors {
		Expect(TagInput{Name: "ok", Color: color}.Validate()).To(Succeed())
	}

	Expect(TagInput{Name: "   ", Color: TagColorRoseMist}.Validate()).
		To(MatchError(ContainSubstring("name is required")))
	Expect(TagInput{Name: strings.Repeat("я", TagNameMaxLen+1), Color: TagColorRoseMist}.Validate()).
		To(MatchError(ContainSubstring("over the 1000 limit")))
	Expect(TagInput{Name: "ok", Color: "#FF0000"}.Validate()).
		To(MatchError(ContainSubstring(`invalid tag color "#FF0000"`)))
	Expect(TagInput{Name: "ok", Color: ""}.Validate()).
		To(MatchError(ContainSubstring("invalid tag color")))
	Expect(TagInput{
		Name: "ok", Color: TagColorRoseMist,
		Description: strings.Repeat("я", TagDescriptionMaxLen+1),
	}.Validate()).To(MatchError(ContainSubstring("over the 5000 limit")))

	// A name at exactly the limit passes; multibyte names are counted in runes,
	// not bytes, so a 1000-character Cyrillic name is not 2000 characters.
	Expect(TagInput{Name: strings.Repeat("я", TagNameMaxLen), Color: TagColorRoseMist}.Validate()).To(Succeed())
}

// TestTagWriteValidationSkipsRequest proves the local checks fail before any
// HTTP call is made.
func TestTagWriteValidationSkipsRequest(t *testing.T) {
	RegisterTestingT(t)
	called := false
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	c := newTestClient(srv)

	_, err := c.CreateTag(context.Background(), TagInput{Name: "", Color: TagColorRoseMist})
	Expect(err).To(HaveOccurred())
	_, err = c.UpdateTag(context.Background(), 1, TagInput{Name: "ok", Color: "nope"})
	Expect(err).To(HaveOccurred())
	_, err = c.UpdateTag(context.Background(), 0, TagInput{Name: "ok", Color: TagColorRoseMist})
	Expect(err).To(MatchError(ContainSubstring("no tag id given")))
	Expect(c.DeleteTag(context.Background(), 0)).To(MatchError(ContainSubstring("no tag id given")))
	Expect(called).To(BeFalse())
}

// TestGetPersonTags locks the per-person read.
func TestGetPersonTags(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodGet))
		Expect(r.URL.Path).To(Equal("/api/v2/persons/p-1/tags"))
		_, _ = io.WriteString(w, tagsListFixture)
	})
	defer srv.Close()

	tags, err := newTestClient(srv).GetPersonTags(context.Background(), "p-1")
	Expect(err).ToNot(HaveOccurred())
	Expect(tags).To(HaveLen(2))
	Expect(tags[0].Name).To(Equal("Проверить в архиве"))
}

// TestAssignPersonTags locks the assign body — a bare JSON array of tag ids —
// and the fact that the endpoint echoes back the person's refreshed tag list.
func TestAssignPersonTags(t *testing.T) {
	RegisterTestingT(t)
	var gotBody []byte
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPost))
		Expect(r.URL.Path).To(Equal("/api/v2/persons/p-1/tags"))
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		gotBody = body
		_, _ = io.WriteString(w, tagsListFixture)
	})
	defer srv.Close()

	tags, err := newTestClient(srv).AssignPersonTags(context.Background(), "p-1",
		[]int{2832, 2833})
	Expect(err).ToNot(HaveOccurred())
	Expect(tags).To(HaveLen(2))
	Expect(tags[0].ID).To(Equal(2832))

	var sent []int
	Expect(json.Unmarshal(gotBody, &sent)).To(Succeed())
	Expect(sent).To(Equal([]int{2832, 2833}))
}

// TestUnassignPersonTags locks the unusual half of the contract: unassigning is
// a DELETE that carries the tag ids as its body.
func TestUnassignPersonTags(t *testing.T) {
	RegisterTestingT(t)
	var gotBody []byte
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodDelete))
		Expect(r.URL.Path).To(Equal("/api/v2/persons/p-1/tags"))
		Expect(r.Header.Get("Content-Type")).To(Equal("application/ld+json"))
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		gotBody = body
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	Expect(newTestClient(srv).UnassignPersonTags(context.Background(), "p-1",
		[]int{2832})).To(Succeed())

	var sent []int
	Expect(json.Unmarshal(gotBody, &sent)).To(Succeed())
	Expect(sent).To(Equal([]int{2832}))
}

// TestPersonTagsWriteGuards proves the empty-argument guards fire before any
// request.
func TestPersonTagsWriteGuards(t *testing.T) {
	RegisterTestingT(t)
	called := false
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	c := newTestClient(srv)
	ctx := context.Background()

	_, err := c.AssignPersonTags(ctx, "p-1", nil)
	Expect(err).To(MatchError(ContainSubstring("no tag ids given")))
	Expect(c.UnassignPersonTags(ctx, "p-1", []int{})).To(MatchError(ContainSubstring("no tag ids given")))
	_, err = c.AssignPersonTags(ctx, "", []int{2832})
	Expect(err).To(MatchError(ContainSubstring("no person uuid given")))
	Expect(c.UnassignPersonTags(ctx, "", []int{2832})).To(MatchError(ContainSubstring("no person uuid given")))
	_, err = c.GetPersonTags(ctx, "")
	Expect(err).To(MatchError(ContainSubstring("no person uuid given")))
	_, err = c.GetTagsByPersons(ctx, nil)
	Expect(err).To(MatchError(ContainSubstring("no person uuids given")))
	Expect(called).To(BeFalse())
}

// TestGetTagsByPersons locks the bulk read: a bare array of person uuids in,
// a map of person uuid to tags out.
func TestGetTagsByPersons(t *testing.T) {
	RegisterTestingT(t)
	var gotBody []byte
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodPost))
		Expect(r.URL.Path).To(Equal("/api/v2/tags/get-by-persons-id-list"))
		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		gotBody = body
		_, _ = io.WriteString(w, `{"p-1": `+tagsListFixture+`, "p-2": []}`)
	})
	defer srv.Close()

	byPerson, err := newTestClient(srv).GetTagsByPersons(context.Background(), []string{"p-1", "p-2"})
	Expect(err).ToNot(HaveOccurred())

	var sent []string
	Expect(json.Unmarshal(gotBody, &sent)).To(Succeed())
	Expect(sent).To(Equal([]string{"p-1", "p-2"}))

	Expect(byPerson).To(HaveLen(2))
	Expect(byPerson["p-1"]).To(HaveLen(2))
	Expect(byPerson["p-1"][1].Name).To(Equal("Раскулачены"))
	Expect(byPerson["p-2"]).To(BeEmpty())
}

// TestPersonTagsDecodesEmptyArray covers the backend quirk that motivates the
// custom unmarshaller: an empty map comes back as [] (PHP's empty associative
// array), which the stock map decoder would reject.
func TestPersonTagsDecodesEmptyArray(t *testing.T) {
	RegisterTestingT(t)

	var empty PersonTags
	Expect(json.Unmarshal([]byte(`[]`), &empty)).To(Succeed())
	Expect(empty).ToNot(BeNil())
	Expect(empty).To(BeEmpty())

	// Whitespace around the empty array is tolerated too.
	Expect(json.Unmarshal([]byte("  []\n"), &empty)).To(Succeed())
	Expect(empty).To(BeEmpty())

	var populated PersonTags
	Expect(json.Unmarshal([]byte(`{"p-1": [{"id": 1, "tag": "Метка", "color": "ice-blue"}]}`),
		&populated)).To(Succeed())
	Expect(populated["p-1"]).To(HaveLen(1))
	Expect(populated["p-1"][0].ID).To(Equal(1))

	// A non-empty array is still an error, not silently swallowed.
	Expect(json.Unmarshal([]byte(`["nope"]`), &populated)).ToNot(Succeed())
}

// TestGetTagsByPersonsEmptyMap proves the quirk survives a real round trip.
func TestGetTagsByPersonsEmptyMap(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	defer srv.Close()

	byPerson, err := newTestClient(srv).GetTagsByPersons(context.Background(), []string{"p-1"})
	Expect(err).ToNot(HaveOccurred())
	Expect(byPerson).To(BeEmpty())
	Expect(byPerson["p-1"]).To(BeEmpty())
}

// TestGetPersonRegularDecodesTagIDs covers the other half of the id-shape
// surprise: the regularPerson view lists a person's tags as bare integer ids,
// not tag objects.
func TestGetPersonRegularDecodesTagIDs(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal("/api/v2/persons/p-1"))
		_, _ = io.WriteString(w, `{"uuid": "p-1", "displayName": "Сандин Яков", "type": "regularPerson",
			"ownerId": "`+testOwnerUUID+`", "gender": "male", "privacyType": "visible_for_all",
			"tags": [2832, 2833]}`)
	})
	defer srv.Close()

	rec, err := newTestClient(srv).GetPersonRegular(context.Background(), "p-1")
	Expect(err).ToNot(HaveOccurred())
	Expect(rec.OwnerID).To(Equal(testOwnerUUID))
	Expect(rec.Tags).To(Equal([]int{2832, 2833}))
}

// TestGetPersonRegularDecodesEmptyTags checks the untagged case, which is the
// one an account with no tags always sees.
func TestGetPersonRegularDecodesEmptyTags(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"uuid": "p-1", "displayName": "Сандин Яков", "tags": []}`)
	})
	defer srv.Close()

	rec, err := newTestClient(srv).GetPersonRegular(context.Background(), "p-1")
	Expect(err).ToNot(HaveOccurred())
	Expect(rec.Tags).To(BeEmpty())
}

// TestTagColorsCoverPalette keeps the ordered list and the hex map in step.
func TestTagColorsCoverPalette(t *testing.T) {
	RegisterTestingT(t)
	Expect(TagColors).To(HaveLen(len(TagColorHex)))
	for _, color := range TagColors {
		Expect(TagColorHex).To(HaveKey(color))
		Expect(TagColorHex[color]).To(HavePrefix("#"))
	}
}
