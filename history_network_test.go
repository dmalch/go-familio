package familio

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// TestListPersonsHistoryLive hits the real change-history endpoints to prove
// the wire types decode against production data. It needs a logged-in Plus
// session (FAMILIO_COOKIES / FAMILIO_SESSION) and is skipped unless
// FAMILIO_NETWORK_TEST=1 so it never runs in CI. It reads a single page to
// bound runtime.
func TestListPersonsHistoryLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	client := newLiveClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	page, err := client.ListPersonsHistory(ctx, HistoryFilter{ItemsPerPage: 20})
	Expect(err).ToNot(HaveOccurred())
	Expect(page.Pager.TotalItems).ToNot(BeZero())
	Expect(page.Data).ToNot(BeEmpty())

	first := page.Data[0]
	Expect(first.Record.ID).ToNot(BeZero())
	Expect(first.Record.Operation).ToNot(BeEmpty())
	Expect(first.Record.PersonDataBlock).ToNot(BeEmpty())
	t.Logf("decoded %d/%d entries; first id=%d %s %s person=%s %s",
		len(page.Data), page.Pager.TotalItems, first.Record.ID, first.Record.Operation,
		first.Record.PersonDataBlock, first.Person.LastName, first.Person.FirstName)

	filters, err := client.GetHistoryFilters(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(filters.Operations).ToNot(BeEmpty())
	t.Logf("facets: %d authors, %d operations, %d data types, %d persons (hasMore=%v)",
		len(filters.Authors), len(filters.Operations), len(filters.DataTypes),
		len(filters.Persons), filters.PersonsHasMore)
}
