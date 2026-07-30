package familio

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// TestListMatchesLive hits the real matches endpoints to prove the wire types
// decode against production data, including the polymorphic foreignPerson. It
// needs a logged-in session and is skipped unless FAMILIO_NETWORK_TEST=1 so it
// never runs in CI. It is deliberately read-only — the confirm/reject/undecide
// endpoints mutate real genealogy data and are covered by the httptest suite
// instead. It reads a single page to bound runtime.
func TestListMatchesLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	client := newLiveClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	page, err := client.ListMatches(ctx, MatchFilter{ItemsPerPage: 5})
	Expect(err).ToNot(HaveOccurred())
	Expect(page.Pager.TotalItems).ToNot(BeZero())
	Expect(page.Data).ToNot(BeEmpty())
	Expect(page.DataVersionMark).ToNot(BeEmpty())

	first := page.Data[0]
	Expect(first.UUID).ToNot(BeEmpty())
	Expect(first.Status).To(BeElementOf(MatchStatusUndecided, MatchStatusConfirmed, MatchStatusRejected))
	Expect(first.OwnPerson.DisplayName).ToNot(BeEmpty())
	Expect(first.ForeignPerson.Type).ToNot(BeEmpty())
	t.Logf("decoded %d/%d matches; first %s score=%d%% status=%s %q vs %q (%s)",
		len(page.Data), page.Pager.TotalItems, first.UUID, first.Score, first.Status,
		first.OwnPerson.DisplayName, first.ForeignPerson.DisplayName, first.ForeignPerson.Type)

	// The scroll cursor is the other paging mode; one hop proves it decodes.
	scroll, err := client.ScrollMatches(ctx, MatchFilter{ItemsPerPage: 2}, "")
	Expect(err).ToNot(HaveOccurred())
	Expect(scroll.Data).ToNot(BeEmpty())
	Expect(scroll.Pager.LastItem).ToNot(BeEmpty())
	next, err := client.ScrollMatches(ctx, MatchFilter{ItemsPerPage: 2}, scroll.Pager.LastItem)
	Expect(err).ToNot(HaveOccurred())
	Expect(next.Data).ToNot(BeEmpty())
	Expect(next.Data[0].UUID).ToNot(Equal(scroll.Data[0].UUID))
	t.Logf("scrolled past %s, hasMore=%v", scroll.Pager.LastItem, next.Pager.HasMore)

	filters, err := client.GetMatchFilters(ctx, MatchFilter{})
	Expect(err).ToNot(HaveOccurred())
	Expect(filters.Statuses).ToNot(BeEmpty())
	t.Logf("facets: %d dates, %d persons, %d users, %d catalogs, %d statuses",
		len(filters.Dates), len(filters.Persons), len(filters.Users),
		len(filters.Catalogs), len(filters.Statuses))
}
