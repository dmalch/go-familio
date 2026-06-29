package familio

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestChildrenOf(t *testing.T) {
	RegisterTestingT(t)
	const person = "uuid-person"
	events := []Event{
		// The person's OWN birth (they are the child) — must NOT count as a child.
		BirthEvent(nil, person, []string{"uuid-grandpa"}, "", ""),
		// Two children whose birth events list the person as a parent.
		BirthEvent(nil, "uuid-son", []string{person, "uuid-spouse"}, "", ""),
		BirthEvent(nil, "uuid-daughter", []string{person}, "", ""),
		// An unrelated birth (person not a participant) — ignored.
		BirthEvent(nil, "uuid-stranger", []string{"uuid-other"}, "", ""),
	}
	Expect(ChildrenOf(events, person)).To(ConsistOf("uuid-daughter", "uuid-son"))
}

func TestSpousesOf(t *testing.T) {
	RegisterTestingT(t)
	const person = "uuid-person"
	events := []Event{
		WeddingEvent(nil, person, "uuid-wife", ""),
		WeddingEvent(nil, "uuid-second-wife", person, ""),
		// A wedding the person is not part of — ignored.
		WeddingEvent(nil, "uuid-a", "uuid-b", ""),
		// Person's own birth — not a wedding, ignored.
		BirthEvent(nil, person, nil, "", ""),
	}
	Expect(SpousesOf(events, person)).To(ConsistOf("uuid-second-wife", "uuid-wife"))
}
