package main

import (
	"testing"

	familio "github.com/dmalch/go-familio"
	. "github.com/onsi/gomega"
)

func ptr[T any](v T) *T { return &v }

// TestBuildPersonView proves person get surfaces derived relations, birth/death
// years, formatted dates, and the marriage uuid on spouses, while keeping the
// raw events.
func TestBuildPersonView(t *testing.T) {
	g := NewWithT(t)

	events := []familio.Event{
		{
			UUID: ptr("b1"), Type: familio.EventBirth,
			Date: familio.EventDate{First: &familio.DatePart{Year: 1826}, Formatted: "1826"},
			Participants: []familio.Participant{
				{PersonUUID: "p", Role: familio.RoleChild, DisplayName: "Иван"},
				{PersonUUID: "dad", Role: familio.RoleParent, DisplayName: "Пётр"},
			},
		},
		{
			UUID: ptr("wed1"), Type: familio.EventWedding,
			Participants: []familio.Participant{
				{PersonUUID: "p", Role: familio.RoleSpouse},
				{PersonUUID: "wife", Role: familio.RoleSpouse, DisplayName: "Анна"},
			},
		},
		{
			UUID: ptr("d1"), Type: familio.EventDeath,
			Date: familio.EventDate{First: &familio.DatePart{Year: 1890}, Formatted: "1890"},
			Participants: []familio.Participant{
				{PersonUUID: "p", Role: familio.RoleOwner},
			},
		},
	}

	v := buildPersonView(&familio.BasicRecord{UUID: "p"}, events, "p")

	g.Expect(*v.BirthYear).To(Equal(1826))
	g.Expect(*v.DeathYear).To(Equal(1890))
	g.Expect(v.BirthDate).To(Equal("1826"))
	g.Expect(v.DeathDate).To(Equal("1890"))
	g.Expect(v.Relations.Parents).To(ConsistOf(familio.PersonRef{UUID: "dad", Name: "Пётр"}))
	g.Expect(v.Relations.Spouses).To(ConsistOf(
		familio.Spouse{UUID: "wife", Name: "Анна", MarriageUUID: "wed1"},
	))
	g.Expect(v.Events).To(HaveLen(3)) // raw events retained
}

// TestBuildPersonView_UnknownYearsOmitted proves the year pointers stay nil (and
// so are omitted from JSON) when there is no dated birth/death event.
func TestBuildPersonView_UnknownYearsOmitted(t *testing.T) {
	g := NewWithT(t)
	events := []familio.Event{
		{
			UUID: ptr("b1"), Type: familio.EventBirth,
			Participants: []familio.Participant{{PersonUUID: "p", Role: familio.RoleChild}},
		},
	}
	v := buildPersonView(&familio.BasicRecord{UUID: "p"}, events, "p")
	g.Expect(v.BirthYear).To(BeNil())
	g.Expect(v.DeathYear).To(BeNil())
}
