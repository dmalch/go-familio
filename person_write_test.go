package familio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
)

// TestUpdatePersonBasicSendsTheVersionHeader locks the optimistic lock: the token
// travels as the X-Base-Version *header*, not a body field (API.md "Update
// basic"), and the body carries only the basic fields.
func TestUpdatePersonBasicSendsTheVersionHeader(t *testing.T) {
	RegisterTestingT(t)
	var gotMethod, gotPath, gotVersion string
	var sent map[string]any

	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotVersion = r.Header.Get("X-Base-Version")
		body, _ := io.ReadAll(r.Body)
		Expect(json.Unmarshal(body, &sent)).To(Succeed())
		_, _ = io.WriteString(w, `{"uuid":"p1","firstName":"Мария","lastName":"Тюжина",
			"middleName":"","birthFirstName":"","birthLastName":"Иванова","gender":"female",
			"privacy":"invisible","createdAt":"2026-01-01T00:00:00+00:00",
			"updatedAt":"2026-07-30T10:00:00+00:00"}`)
	})
	defer srv.Close()

	rec, err := newTestClient(srv).UpdatePersonBasic(context.Background(), "p1", BasicFields{
		FirstName:     "Мария",
		LastName:      "Тюжина",
		BirthLastName: "Иванова",
		Gender:        GenderFemale,
		Privacy:       PrivacyInvisible,
	}, "2026-07-14T21:37:12+00:00")
	Expect(err).ToNot(HaveOccurred())

	Expect(gotMethod).To(Equal(http.MethodPut))
	Expect(gotPath).To(Equal("/api/v2/persons/p1/basic"))
	Expect(gotVersion).To(Equal("2026-07-14T21:37:12+00:00"))

	Expect(sent["firstName"]).To(Equal("Мария"))
	Expect(sent["birthLastName"]).To(Equal("Иванова"))
	Expect(sent["gender"]).To(Equal("female"))
	Expect(sent["privacy"]).To(Equal("invisible"))
	Expect(sent).ToNot(HaveKey("uuid"), "the body is just the basic fields")

	// The response's bumped updatedAt is the token for the *next* edit.
	Expect(rec.UpdatedAt).To(Equal("2026-07-30T10:00:00+00:00"))
	Expect(rec.LastName).To(Equal("Тюжина"))
}

// TestUpdatePersonBasicStaleVersionIsAConflict pins the 409 a stale token gets.
func TestUpdatePersonBasicStaleVersionIsAConflict(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	defer srv.Close()

	_, err := newTestClient(srv).UpdatePersonBasic(context.Background(), "p1",
		BasicFields{FirstName: "Мария", Gender: GenderFemale}, "stale")
	Expect(err).To(MatchError(ErrConflict))
}

// TestDeletePersonReturns204 covers the person delete.
func TestDeletePersonReturns204(t *testing.T) {
	RegisterTestingT(t)
	var gotMethod, gotPath string
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	Expect(newTestClient(srv).DeletePerson(context.Background(), "p1")).To(Succeed())
	Expect(gotMethod).To(Equal(http.MethodDelete))
	Expect(gotPath).To(Equal("/api/v2/persons/p1"))
}

// TestDeletePersonAlreadyGone keeps the 404 mapping so idempotent deletes are easy.
func TestDeletePersonAlreadyGone(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	Expect(newTestClient(srv).DeletePerson(context.Background(), "gone")).To(MatchError(ErrNotFound))
}

// TestGetPersonDisplay reads the computed display name off the regularPerson view
// — the name a /basic read does not carry.
func TestGetPersonDisplay(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal("/api/v2/persons/p1"))
		_, _ = io.WriteString(w, `{"uuid":"p1","type":"regularPerson",
			"displayName":"Тюжин Леонтий Епифанович","ownerId":"`+testOwnerUUID+`"}`)
	})
	defer srv.Close()

	display, err := newTestClient(srv).GetPersonDisplay(context.Background(), "p1")
	Expect(err).ToNot(HaveOccurred())
	Expect(display.UUID).To(Equal("p1"))
	Expect(display.DisplayName).To(Equal("Тюжин Леонтий Епифанович"))
}

// TestCreatePersonSendsOwnerAndSelfBirth locks the create envelope: the ?owner=
// query taken from the JWT uuid claim, the nested "basic", and the self-referencing
// birth event familio requires.
func TestCreatePersonSendsOwnerAndSelfBirth(t *testing.T) {
	RegisterTestingT(t)
	var gotQuery, gotPath string
	var sent map[string]any

	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		Expect(json.Unmarshal(body, &sent)).To(Succeed())

		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"basic":{"uuid":"new-1","displayName":"Иванов Иван",
			"firstName":"Иван","lastName":"Иванов","gender":"male",
			"createdAt":"2026-07-30T10:00:00+00:00","updatedAt":"2026-07-30T10:00:00+00:00"},
			"photo":null,
			"events":[{"uuid":"ev-birth","type":"birth",
			  "date":{"calendar":"gregorian","type":"equal","first":{"year":1890},"formatted":"1890"},
			  "participants":[{"personUuid":"new-1","role":"child"}]}]}`)
	})
	defer srv.Close()

	created, err := newTestClient(srv).CreatePerson(context.Background(), CreatePersonInput{
		Basic: BasicFields{FirstName: "Иван", LastName: "Иванов", Gender: GenderMale, Privacy: PrivacyVisibleForAll},
		Events: []Event{
			SelfBirthEvent(&DateRange{Year: 1890}, "", ""),
		},
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(gotPath).To(Equal("/api/v2/persons"))
	Expect(gotQuery).To(Equal("owner="+testOwnerUUID), "the create must be attributed to the account")

	basic := asMap(sent["basic"])
	Expect(basic["firstName"]).To(Equal("Иван"))
	events := asSlice(sent["events"])
	Expect(events).To(HaveLen(1))
	birth := asMap(events[0])
	Expect(birth["type"]).To(Equal("birth"))
	participant := asMap(asSlice(birth["participants"])[0])
	Expect(participant["personUuid"]).To(Equal(SelfRef), "the new person refers to itself as \"self\"")
	Expect(participant["role"]).To(Equal("child"))

	Expect(created.Basic.UUID).To(Equal("new-1"))
	Expect(created.Basic.DisplayName).To(Equal("Иванов Иван"))
	Expect(created.Events).To(HaveLen(1))
	Expect(created.Events[0].ID()).To(Equal("ev-birth"))
}

// TestSelfEventBuilders check the create-time helpers anchor on SelfRef with the
// role each event type demands: a birth's subject is a "child", a death's and a
// baptism's is the "owner".
func TestSelfEventBuilders(t *testing.T) {
	RegisterTestingT(t)

	birth := SelfBirthEvent(&DateRange{Year: 1890}, "settlement-1", "заметка")
	Expect(birth.Type).To(Equal("birth"))
	Expect(birth.Participants).To(HaveLen(1))
	Expect(birth.Participants[0].PersonUUID).To(Equal(SelfRef))
	Expect(birth.Participants[0].Role).To(Equal(RoleChild))
	Expect(birth.SettlementUUID()).To(Equal("settlement-1"))
	Expect(birth.Comment).To(Equal("заметка"))

	death := SelfDeathEvent(&DateRange{Year: 1942}, "", "")
	Expect(death.Type).To(Equal("death"))
	Expect(death.Participants[0].PersonUUID).To(Equal(SelfRef))
	Expect(death.Participants[0].Role).To(Equal(RoleOwner))
	Expect(death.Settlement).To(BeNil(), "an empty place must be null, not an empty object")

	baptism := SelfBaptismEvent(nil, "", "")
	Expect(baptism.Type).To(Equal("baptism"))
	Expect(baptism.Participants[0].PersonUUID).To(Equal(SelfRef))
	Expect(baptism.Participants[0].Role).To(Equal(RoleOwner))
}
