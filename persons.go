package familio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// FlexDate tolerates familio's heterogeneous date encodings: a JSON null, a
// plain string, or a structured object {type, calendar, first, second,
// formatted}. It exposes the human-readable "formatted" form.
type FlexDate struct {
	Formatted string
	Present   bool
}

func (d *FlexDate) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	// Plain string form.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		d.Formatted, d.Present = s, true
		return nil
	}
	// Structured object form — keep the formatted field.
	var obj struct {
		Formatted string `json:"formatted"`
	}
	if err := json.Unmarshal(b, &obj); err == nil {
		d.Formatted, d.Present = obj.Formatted, true
		return nil
	}
	// Unknown shape: ignore rather than failing the whole list.
	return nil
}

// Value returns the formatted date and whether a non-empty value is present.
func (d FlexDate) Value() (string, bool) {
	return d.Formatted, d.Present && d.Formatted != ""
}

// settlementPageSize is the per-page size for the settlement-persons sweep.
// familio's backend accepts at least 500 but times out on very large pages;
// 300 with pagination is the documented safe value.
const settlementPageSize = 300

// Person is one entry from GET /api/v2/persons. The data array is heterogeneous:
// catalog persons carry catalogKey/catalogName, while user-created tree persons
// instead carry gender/birthPlace/ownerId/etc. CatalogKey is a pointer so a
// JSON null (tree person) is distinguishable from an empty string; birth/death
// dates use FlexDate because familio encodes them as null, a string, or an
// object depending on the record.
type Person struct {
	UUID                string   `json:"uuid"`
	DisplayName         string   `json:"displayName"`
	ShortDisplayName    string   `json:"shortDisplayName"`
	CatalogKey          *string  `json:"catalogKey"`
	CatalogName         string   `json:"catalogName"`
	Type                string   `json:"type"`
	Gender              string   `json:"gender"`
	BirthDate           FlexDate `json:"birthDate"`
	DeathDate           FlexDate `json:"deathDate"`
	HasDeathEvent       bool     `json:"hasDeathEvent"`
	BirthSettlementText string   `json:"birthSettlementText"`
	UpdatedAt           string   `json:"updatedAt"`
}

// personsPage is the envelope returned by GET /api/v2/persons.
type personsPage struct {
	Pager struct {
		Page         int `json:"page"`
		ItemsPerPage int `json:"itemsPerPage"`
		TotalItems   int `json:"totalItems"`
	} `json:"pager"`
	Data []Person `json:"data"`
}

// ListSettlementPersons returns every person (catalog-sourced + user-created)
// linked to a settlement, paging through GET /api/v2/persons?settlement=<uuid>.
// This is the one fully-known, public endpoint and the provider's working read
// path. Callers filter by CatalogKey client-side (there is no server-side
// catalog facet).
func (c *Client) ListSettlementPersons(ctx context.Context, settlement string) ([]Person, error) {
	var all []Person
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("settlement", settlement)
		q.Set("itemsPerPage", strconv.Itoa(settlementPageSize))
		q.Set("page", strconv.Itoa(page))

		req, err := c.newRequest(ctx, http.MethodGet, "persons", q, nil)
		if err != nil {
			return nil, err
		}

		var pg personsPage
		if err := c.do(req, &pg); err != nil {
			return nil, err
		}

		all = append(all, pg.Data...)

		// Stop on a short/empty page or once we've collected everything.
		if len(pg.Data) < settlementPageSize ||
			(pg.Pager.TotalItems > 0 && len(all) >= pg.Pager.TotalItems) {
			break
		}
	}
	return all, nil
}
