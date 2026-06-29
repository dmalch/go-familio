package familio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

// biographyTestServer stands up an httptest server that serves a scrape-able
// bearer token at the root (so the authed paths get past bearerToken) and
// delegates every other path to handler.
func biographyTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"eyJ.eyJ.sig"}}</script>`)
			return
		}
		handler(w, r)
	}))
}

func newBiographyClient(srv *httptest.Server) *Client {
	c, _ := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookiesFromHeader("t=secret"),
		RateLimit: 1000,
	})
	return c
}

// TestGetPersonBiography reads the dedicated /biography sub-resource and decodes
// its {text, updatedAt} envelope (text in `text`, the resource's own version in
// `updatedAt`).
func TestGetPersonBiography(t *testing.T) {
	RegisterTestingT(t)
	srv := biographyTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodGet))
		Expect(r.URL.Path).To(Equal("/api/v2/persons/p1/biography"))
		_, _ = io.WriteString(w, `{"text":"Жил-был человек","updatedAt":"2024-07-02T21:20:37+00:00"}`)
	})
	defer srv.Close()

	bio, err := newBiographyClient(srv).GetPersonBiography(context.Background(), "p1")
	Expect(err).ToNot(HaveOccurred())
	Expect(bio.Text).To(Equal("Жил-был человек"))
	Expect(bio.UpdatedAt).To(Equal("2024-07-02T21:20:37+00:00"))
}

// TestUpdatePersonBiography locks the write contract: PUT /biography with a
// {"text":…} body and the biography's own updatedAt in X-Base-Version; the
// refreshed {text, updatedAt} is returned.
func TestUpdatePersonBiography(t *testing.T) {
	RegisterTestingT(t)
	var gotMethod, gotVersion string
	var gotBody map[string]any
	srv := biographyTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotVersion = r.Header.Get("X-Base-Version")
		Expect(r.URL.Path).To(Equal("/api/v2/persons/p1/biography"))
		Expect(json.NewDecoder(r.Body).Decode(&gotBody)).To(Succeed())
		_, _ = io.WriteString(w, `{"text":"Новое","updatedAt":"2026-06-29T13:35:43+00:00"}`)
	})
	defer srv.Close()

	bio, err := newBiographyClient(srv).UpdatePersonBiography(
		context.Background(), "p1", "Новое", "2024-07-02T21:20:37+00:00")
	Expect(err).ToNot(HaveOccurred())
	Expect(gotMethod).To(Equal(http.MethodPut))
	Expect(gotVersion).To(Equal("2024-07-02T21:20:37+00:00"))
	Expect(gotBody).To(Equal(map[string]any{"text": "Новое"}))
	Expect(bio.Text).To(Equal("Новое"))
	Expect(bio.UpdatedAt).To(Equal("2026-06-29T13:35:43+00:00"))
}

// TestCreatePersonSendsBiography proves the create envelope carries the optional
// biography string from CreatePersonInput.
func TestCreatePersonSendsBiography(t *testing.T) {
	RegisterTestingT(t)
	var gotBody map[string]any
	srv := biographyTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal("/api/v2/persons"))
		Expect(json.NewDecoder(r.Body).Decode(&gotBody)).To(Succeed())
		_, _ = io.WriteString(w, `{"basic":{"uuid":"new-uuid"},"events":[]}`)
	})
	defer srv.Close()

	bio := "Краткая справка"
	resp, err := newBiographyClient(srv).CreatePerson(context.Background(), CreatePersonInput{
		Basic:     BasicFields{FirstName: "Иван", Gender: GenderMale},
		Biography: &bio,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(resp.Basic.UUID).To(Equal("new-uuid"))
	Expect(gotBody["biography"]).To(Equal("Краткая справка"))
}
