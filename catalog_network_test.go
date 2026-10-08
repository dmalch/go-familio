package familio

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// TestCatalogPersonLive reads a real catalog record and its catalog anonymously —
// both are public — and checks they decode: the WWI casualty-list entry for
// «Мальчиков Михаил» that the person search surfaced.
func TestCatalogPersonLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	client, err := NewClient(Options{RateLimit: 1000})
	Expect(err).ToNot(HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := client.GetCatalogPerson(ctx, "gwarmil", "774b6dcb-b44a-4304-944b-d7a5d4513c61")
	Expect(err).ToNot(HaveOccurred(), "live catalog record read failed")
	Expect(p.CatalogKey).To(Equal("gwarmil"))
	Expect(p.Text).To(ContainSubstring("Мальчиков"))
	Expect(p.Record).To(HaveKey("record_text"))

	c, err := client.GetCatalog(ctx, p.CatalogKey)
	Expect(err).ToNot(HaveOccurred(), "live catalog read failed")
	Expect(c.Key).To(Equal("gwarmil"))
	Expect(c.Fields).ToNot(BeEmpty())
	t.Logf("%s: %q, %d fields, %d records; record %q in %v",
		c.Key, c.Name, len(c.Fields), c.RecordsCount, p.Text, p.Geography)
}
