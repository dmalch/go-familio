package familio

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// TestListSettlementPersonsLive hits the real familio.org endpoint to prove the
// wire types decode against production data (incl. heterogeneous catalog/tree
// records and object-shaped dates). Skipped unless FAMILIO_NETWORK_TEST=1 so it
// never runs in CI. To bound runtime against huge settlements, it reads a single
// page via a one-shot client rather than the full ListSettlementPersons sweep.
func TestListSettlementPersonsLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	const zhuravkino = "e0c1a09c-b7ed-4d5c-a22f-3a86db42bbc6"

	client, err := NewClient(Options{RateLimit: 1000})
	Expect(err).ToNot(HaveOccurred())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := url.Values{}
	q.Set("settlement", zhuravkino)
	q.Set("itemsPerPage", "300")
	q.Set("page", "1")
	req, err := client.newRequest(ctx, http.MethodGet, "persons", q, nil)
	Expect(err).ToNot(HaveOccurred())

	var page personsPage
	Expect(client.do(req, &page)).To(Succeed(), "live decode failed")

	Expect(page.Pager.TotalItems).ToNot(BeZero())
	Expect(page.Data).ToNot(BeEmpty())
	t.Logf("decoded %d/%d persons; sample uuid=%s display=%q catalogKey=%v birthDate=%q",
		len(page.Data), page.Pager.TotalItems, page.Data[0].UUID,
		page.Data[0].DisplayName, page.Data[0].CatalogKey, page.Data[0].BirthDate.Formatted)
}
