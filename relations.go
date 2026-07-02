package familio

import "slices"

// PersonRef is a minimal reference to a related person: the full uuid and, when
// an event carried it, a display name. UUID is always the complete uuid (never a
// truncated prefix) so downstream tooling can key on it safely.
type PersonRef struct {
	UUID string `json:"uuid"`
	Name string `json:"name,omitempty"`
}

// Spouse is a spouse reference plus the uuid of the underlying wedding event —
// familio's "union"/marriage identity, needed to import a familio_marriage or to
// target that marriage for deletion. familio has no separate union resource; the
// wedding event uuid *is* the marriage uuid.
type Spouse struct {
	UUID         string `json:"uuid"`
	Name         string `json:"name,omitempty"`
	MarriageUUID string `json:"marriageUuid,omitempty"`
}

// Relations is the normalized kinship of one person, derived from their events:
// parents/spouses/children as flat reference lists rather than per-event
// participant roles. It is the convenience view every consumer would otherwise
// reconstruct from events[].participants[] by hand.
type Relations struct {
	Parents  []PersonRef `json:"parents"`
	Spouses  []Spouse    `json:"spouses"`
	Children []PersonRef `json:"children"`
}

// displayNames builds a uuid → display name map from every participant across
// the events, keeping the first non-empty name seen for each uuid. familio's
// /events participants carry displayName, so related persons can be named
// without an extra fetch per relative.
func displayNames(events []Event) map[string]string {
	names := make(map[string]string)
	for i := range events {
		for _, p := range events[i].Participants {
			if p.DisplayName != "" {
				if _, ok := names[p.PersonUUID]; !ok {
					names[p.PersonUUID] = p.DisplayName
				}
			}
		}
	}
	return names
}

// DeriveRelations reduces a person's events into normalized parents, spouses,
// and children. Names come from the events' participant display names when
// present. Spouses carry the wedding event uuid as their MarriageUUID. All uuids
// are full uuids.
func DeriveRelations(events []Event, personUUID string) Relations {
	names := displayNames(events)
	ref := func(uuid string) PersonRef { return PersonRef{UUID: uuid, Name: names[uuid]} }

	rel := Relations{
		Parents:  []PersonRef{},
		Spouses:  []Spouse{},
		Children: []PersonRef{},
	}

	if birth := OwnBirthEvent(events, personUUID); birth != nil {
		for _, uuid := range birth.ParentUUIDs() {
			rel.Parents = append(rel.Parents, ref(uuid))
		}
	}

	for _, uuid := range ChildrenOf(events, personUUID) {
		rel.Children = append(rel.Children, ref(uuid))
	}

	// Spouses, with the wedding event uuid as the marriage identity.
	for i := range events {
		if events[i].Type != EventWedding {
			continue
		}
		spouses := events[i].SpouseUUIDs()
		if !slices.Contains(spouses, personUUID) {
			continue
		}
		for _, uuid := range spouses {
			if uuid == personUUID {
				continue
			}
			rel.Spouses = append(rel.Spouses, Spouse{
				UUID:         uuid,
				Name:         names[uuid],
				MarriageUUID: events[i].ID(),
			})
		}
	}

	return rel
}

// OwnDeathEvent returns the death event owned by personUUID, or nil. A person's
// /events lists only their own death (death is single-subject, one per person),
// so a plain type filter suffices; this mirrors OwnBirthEvent for symmetry.
func OwnDeathEvent(events []Event, personUUID string) *Event {
	for i := range events {
		if events[i].Type != EventDeath {
			continue
		}
		for _, p := range events[i].Participants {
			if p.Role == RoleOwner && p.PersonUUID == personUUID {
				return &events[i]
			}
		}
	}
	return nil
}

// eventYear returns the primary year of an event's date and whether it is known
// (a nil First means the date is unknown).
func eventYear(ev *Event) (int, bool) {
	if ev == nil || ev.Date.First == nil {
		return 0, false
	}
	return ev.Date.First.Year, true
}

// BirthYear returns personUUID's birth year (from their own birth event) and
// whether it is known.
func BirthYear(events []Event, personUUID string) (int, bool) {
	return eventYear(OwnBirthEvent(events, personUUID))
}

// DeathYear returns personUUID's death year (from their own death event) and
// whether it is known.
func DeathYear(events []Event, personUUID string) (int, bool) {
	return eventYear(OwnDeathEvent(events, personUUID))
}
