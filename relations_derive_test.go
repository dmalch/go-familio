package familio

import (
	"testing"

	. "github.com/onsi/gomega"
)

// namedParticipant builds a participant carrying a display name, so the derived
// relations can be named without an extra fetch.
func namedParticipant(uuid, role, name string) Participant {
	return Participant{PersonUUID: uuid, Role: role, DisplayName: name}
}

// yearEvent builds an event of the given type with a known primary year and the
// supplied participants.
func yearEvent(eventType string, year int, parts ...Participant) Event {
	uuid := eventType
	return Event{
		UUID:         &uuid,
		Type:         eventType,
		Date:         EventDate{First: &DatePart{Year: year}, Formatted: "формат"},
		Participants: parts,
	}
}

func personRelationsFixture() []Event {
	const person = "p"
	wedUUID := "wedding-uuid"
	return []Event{
		yearEvent(EventBirth, 1826,
			namedParticipant(person, RoleChild, "Иван"),
			namedParticipant("dad", RoleParent, "Пётр"),
			namedParticipant("mom", RoleParent, "Мария"),
		),
		{
			UUID: &wedUUID, Type: EventWedding,
			Participants: []Participant{
				namedParticipant(person, RoleSpouse, "Иван"),
				namedParticipant("wife", RoleSpouse, "Анна"),
			},
		},
		yearEvent(EventBirth, 1850,
			namedParticipant("son", RoleChild, "Сын"),
			namedParticipant(person, RoleParent, "Иван"),
		),
		yearEvent(EventDeath, 1890, namedParticipant(person, RoleOwner, "Иван")),
	}
}

func TestDeriveRelations(t *testing.T) {
	RegisterTestingT(t)
	rel := DeriveRelations(personRelationsFixture(), "p")

	Expect(rel.Parents).To(ConsistOf(
		PersonRef{UUID: "dad", Name: "Пётр"},
		PersonRef{UUID: "mom", Name: "Мария"},
	))
	Expect(rel.Children).To(ConsistOf(PersonRef{UUID: "son", Name: "Сын"}))
	Expect(rel.Spouses).To(ConsistOf(Spouse{UUID: "wife", Name: "Анна", MarriageUUID: "wedding-uuid"}))
}

// TestDeriveRelationsAlwaysNonNilSlices proves the relation slices are empty
// (not null) for a person with no relations, so JSON output is stable arrays.
func TestDeriveRelationsAlwaysNonNilSlices(t *testing.T) {
	RegisterTestingT(t)
	rel := DeriveRelations(nil, "p")
	Expect(rel.Parents).ToNot(BeNil())
	Expect(rel.Spouses).ToNot(BeNil())
	Expect(rel.Children).ToNot(BeNil())
}

func TestBirthDeathYear(t *testing.T) {
	RegisterTestingT(t)
	events := personRelationsFixture()

	by, ok := BirthYear(events, "p")
	Expect(ok).To(BeTrue())
	Expect(by).To(Equal(1826))

	dy, ok := DeathYear(events, "p")
	Expect(ok).To(BeTrue())
	Expect(dy).To(Equal(1890))

	_, ok = DeathYear(events, "son")
	Expect(ok).To(BeFalse())
}

func TestOwnDeathEvent(t *testing.T) {
	RegisterTestingT(t)
	events := personRelationsFixture()
	Expect(OwnDeathEvent(events, "p")).ToNot(BeNil())
	Expect(OwnDeathEvent(events, "wife")).To(BeNil())
}
