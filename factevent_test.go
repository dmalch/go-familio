package familio

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestFactEvent(t *testing.T) {
	RegisterTestingT(t)
	ev := FactEvent("location", &DateRange{Year: 1880}, "person-uuid", "Москва")
	Expect(ev.Type).To(Equal("location"))
	Expect(ev.Comment).To(Equal("Москва"))
	Expect(ev.Participants).To(HaveLen(1))
	Expect(ev.Participants[0].Role).To(Equal(RoleOwner))
	Expect(ev.Participants[0].PersonUUID).To(Equal("person-uuid"))
}

func TestFactEventTypesExcludesManagedElsewhere(t *testing.T) {
	RegisterTestingT(t)
	Expect(FactEventTypes).ToNot(ContainElements("birth", "death", "baptism", "wedding", "divorce"),
		"types managed by a dedicated surface must not be fact events")
}

func TestFactEventTypesIncludesGodparentAndWarranter(t *testing.T) {
	RegisterTestingT(t)
	// Per Familio's own model these are single-subject events on the
	// godparent/witness, so familio_event must accept them.
	Expect(FactEventTypes).To(ContainElements("godparent", "warranter"))
}
