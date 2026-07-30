package familio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
)

// TestCreateEventPostsUnderThePerson locks the event write: the ld+json content
// type API-Platform requires, the person-scoped path (events are strictly a
// person sub-resource — POST /api/v2/events is a 404), and the decode of the
// server-assigned uuid.
func TestCreateEventPostsUnderThePerson(t *testing.T) {
	RegisterTestingT(t)
	var gotMethod, gotPath, gotContentType, gotAccept string
	var sent map[string]any

	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		Expect(json.Unmarshal(body, &sent)).To(Succeed())

		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"uuid":"ev-1","type":"wedding",
			"date":{"calendar":"gregorian","type":"equal","first":{"year":1914,"month":10,"day":5},
			        "formatted":"5 октября 1914"},
			"participants":[{"personUuid":"pa","role":"spouse","displayName":"Леонтий","gender":"male"},
			                {"personUuid":"pb","role":"spouse","displayName":"Мария","gender":"female"}],
			"comment":"МК Журавкино"}`)
	})
	defer srv.Close()

	month, day := 10, 5
	ev, err := newTestClient(srv).CreateEvent(context.Background(), "pa",
		WeddingEvent(&DateRange{Year: 1914, Month: &month, Day: &day}, "pa", "pb", "МК Журавкино"))
	Expect(err).ToNot(HaveOccurred())

	Expect(gotMethod).To(Equal(http.MethodPost))
	Expect(gotPath).To(Equal("/api/v2/persons/pa/events"))
	Expect(gotContentType).To(Equal("application/ld+json"))
	Expect(gotAccept).To(Equal("application/ld+json"))

	// The request must carry a null uuid (a create) and both spouses.
	Expect(sent["uuid"]).To(BeNil())
	Expect(sent["type"]).To(Equal("wedding"))
	Expect(sent["participants"]).To(HaveLen(2))

	Expect(ev.ID()).To(Equal("ev-1"))
	Expect(ev.SpouseUUIDs()).To(ConsistOf("pa", "pb"))
	Expect(ev.Comment).To(Equal("МК Журавкино"))
}

// TestDeleteEventAcceptsAnyParticipantAsAnchor covers the 204 delete and the fact
// that the anchor person need not be the event's "owner".
func TestDeleteEventAcceptsAnyParticipantAsAnchor(t *testing.T) {
	RegisterTestingT(t)
	var gotMethod, gotPath string
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	// pb is the *second* spouse: deleting through them must work the same.
	err := newTestClient(srv).DeleteEvent(context.Background(), "pb", "ev-1")
	Expect(err).ToNot(HaveOccurred())
	Expect(gotMethod).To(Equal(http.MethodDelete))
	Expect(gotPath).To(Equal("/api/v2/persons/pb/events/ev-1"))
}

// TestDeleteEventOnTheSoleBirthIsAConflict pins the documented 409: birth is
// mandatory, so it is upsert-only (see API.md "Two event classes").
func TestDeleteEventOnTheSoleBirthIsAConflict(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"message":"Нельзя удалить единственное событие рождения"}`)
	})
	defer srv.Close()

	err := newTestClient(srv).DeleteEvent(context.Background(), "p1", "birth-1")
	Expect(err).To(MatchError(ErrConflict))
}

// TestCreateEventNotFoundOnAMissingPerson keeps the 404 mapping on the write path.
func TestCreateEventNotFoundOnAMissingPerson(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	_, err := newTestClient(srv).CreateEvent(context.Background(), "gone",
		FactEvent("location", nil, "gone", ""))
	Expect(err).To(MatchError(ErrNotFound))
}

// TestFindByID locates an event in a person's list and reports a miss as nil.
func TestFindByID(t *testing.T) {
	RegisterTestingT(t)
	e1, e2 := "e1", "e2"
	events := []Event{
		{UUID: &e1, Type: "birth"},
		{UUID: &e2, Type: "wedding"},
	}
	Expect(FindByID(events, "e2").Type).To(Equal("wedding"))
	Expect(FindByID(events, "nope")).To(BeNil())
	Expect(FindByID(nil, "e1")).To(BeNil())
}
