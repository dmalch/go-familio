package familio

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// TestListTagsLive hits the real tags endpoints to prove the wire types decode
// against production data. It needs a logged-in session and is skipped unless
// FAMILIO_NETWORK_TEST=1 so it never runs in CI. It is deliberately read-only —
// create/update/delete and the person assign/unassign calls mutate the real
// account and are covered by the httptest suite instead.
//
// An account may legitimately own no tags (they are a Familio Plus feature), so
// the assertions check that the reads succeed and decode, not that they return
// data.
func TestListTagsLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	client := newLiveClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tags, err := client.ListTags(ctx)
	Expect(err).ToNot(HaveOccurred())
	for _, tag := range tags {
		Expect(tag.ID).To(BeNumerically(">", 0))
		Expect(tag.Name).ToNot(BeEmpty())
		Expect(TagColorHex).To(HaveKey(tag.Color),
			"unknown colour %q — the palette in tags.go is out of date", tag.Color)
	}
	t.Logf("decoded %d tag(s)", len(tags))

	// The per-person and bulk reads need a person uuid; take one of the
	// account's own persons from the matches facets.
	person := liveOwnPersonUUID(ctx, t, client)
	if person == "" {
		t.Log("no own person uuid available; skipping the person-level tag reads")
		return
	}

	assigned, err := client.GetPersonTags(ctx, person)
	Expect(err).ToNot(HaveOccurred())
	t.Logf("person %s has %d tag(s)", person, len(assigned))

	// The bulk read is the one that decodes the map-or-empty-array shape.
	byPerson, err := client.GetTagsByPersons(ctx, []string{person})
	Expect(err).ToNot(HaveOccurred())
	Expect(byPerson).ToNot(BeNil())
	Expect(byPerson[person]).To(HaveLen(len(assigned)))
	t.Logf("bulk read returned %d person entr(ies)", len(byPerson))
}

// liveOwnPersonUUID picks one of the account's own person uuids off the matches
// person facet, or returns "" when the account has no matches to draw from.
func liveOwnPersonUUID(ctx context.Context, t *testing.T, client *Client) string {
	t.Helper()
	filters, err := client.GetMatchFilters(ctx, MatchFilter{})
	if err != nil {
		t.Logf("could not read the matches facets for a person uuid: %v", err)
		return ""
	}
	for _, facet := range filters.Persons {
		if facet.Item.Value != "" {
			return facet.Item.Value
		}
	}
	return ""
}
