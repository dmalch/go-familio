package familio

import (
	"context"
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
)

// profileFixture is the shape GET /api/v2/profile returns: the account (user)
// and the account holder's own person-like details (profile). See API.md
// "Persons — authed read".
const profileFixture = `{
  "user": {"uuid": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0", "email": "dmalch@gmail.com"},
  "profile": {"displayName": "Мальчиков Д.", "firstName": "Дмитрий",
              "lastName": "Мальчиков", "middleName": "Сергеевич", "gender": "male"}
}`

// TestGetProfile locks the two-level decode: the account's uuid and email live
// under "user", the display fields under "profile".
func TestGetProfile(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodGet))
		Expect(r.URL.Path).To(Equal("/api/v2/profile"))
		Expect(r.Header.Get("Authorization")).To(HavePrefix("Bearer eyJ"))
		_, _ = io.WriteString(w, profileFixture)
	})
	defer srv.Close()

	profile, err := newTestClient(srv).GetProfile(context.Background())
	Expect(err).ToNot(HaveOccurred())

	Expect(profile.User.UUID).To(Equal(testOwnerUUID))
	Expect(profile.User.Email).To(Equal("dmalch@gmail.com"))
	Expect(profile.Details.DisplayName).To(Equal("Мальчиков Д."))
	Expect(profile.Details.FirstName).To(Equal("Дмитрий"))
	Expect(profile.Details.LastName).To(Equal("Мальчиков"))
	Expect(profile.Details.MiddleName).To(Equal("Сергеевич"))
	Expect(profile.Details.Gender).To(Equal(GenderMale))
}

// TestGetProfileUnauthenticated proves the endpoint is bearer-gated: a session
// that yields no token surfaces as ErrNotLoggedIn rather than a decode failure.
func TestGetProfileUnauthenticated(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Требуется авторизация"}`)
	})
	defer srv.Close()

	_, err := newTestClient(srv).GetProfile(context.Background())
	Expect(err).To(MatchError(ErrNotLoggedIn))
}
