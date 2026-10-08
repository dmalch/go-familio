package familio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Catalog is one of familio's record catalogs («справочники»): a digitised
// people index such as a WWI casualty list or a parish's metric books
// (GET /api/v1/catalogs/<key or uuid>). Fields describes the catalog-specific
// keys of its records' CatalogPerson.Record, in display order, with the titles
// the site labels them with.
type Catalog struct {
	UUID         string         `json:"uuid"`
	Key          string         `json:"key"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Years        string         `json:"years"` // the period covered, e.g. "1854-1876"
	Hidden       bool           `json:"hidden"`
	Type         string         `json:"type"` // familio's catalog kind, e.g. "fundSettlements"
	RecordsCount int            `json:"recordsCount"`
	Fields       []CatalogField `json:"fields"`
}

// CatalogField describes one key of a catalog's records: its title
// («Дата крещения» for baptism_date), its type, and where the site shows it.
// A HiddenFromUnauthorized field reaches only a client with a session.
type CatalogField struct {
	Key                    string `json:"key"`
	Title                  string `json:"title"`
	Type                   string `json:"type"` // "string" or "text"
	ShowInCard             bool   `json:"showInCard"`
	ShowInTable            bool   `json:"showInTable"`
	HiddenFromUnauthorized bool   `json:"hiddenFromUnauthorized"`
}

// CatalogPerson is one catalog record — an entry that SearchPersons returns as
// a "catalogPerson" (GET /api/v1/catalogs/<catalogKey>/excerpts/<uuid>; familio
// calls it an excerpt). Attach it to a tree person with CreateSource, as a
// SourceTypeCatalogPerson with this CatalogKey and UUID.
type CatalogPerson struct {
	UUID       string `json:"uuid"`
	CatalogKey string `json:"catalogKey"`
	RecordID   string `json:"recordId"` // the catalog row this excerpt was taken from
	Text       string `json:"text"`     // the name as the record has it

	BirthDate *EventDate `json:"birthDate"` // nil when the record has none
	DeathDate *EventDate `json:"deathDate"`

	// Geography is the record's place as written ("Тобольская губерния, …"),
	// and BirthPlace the birth place when the catalog has one. Both come from
	// Attributes, which holds every searchable attribute familio derived,
	// dates included.
	Geography  []string       `json:"geography,omitempty"`
	BirthPlace string         `json:"birthPlace,omitempty"`
	Attributes map[string]any `json:"attributes"`

	// Record is the catalog-specific row as sent: its keys depend on the
	// catalog (Catalog.Fields titles them), and some values carry HTML — line
	// breaks, emphasis, links to familio's settlement pages.
	Record map[string]any `json:"record"`

	LinkedPersons []CatalogLinkedPerson `json:"linkedPersons"` // tree persons citing the record
	Settlements   []CatalogSettlement   `json:"settlements"`   // settlements familio bound the record to
}

// CatalogLinkedPerson is a tree person that cites a catalog record.
type CatalogLinkedPerson struct {
	UUID       string `json:"uuid"`
	LastName   string `json:"lastName"`
	FirstName  string `json:"firstName"`
	MiddleName string `json:"middleName"`
	Gender     string `json:"gender"` // GenderMale, GenderFemale, or "" when unknown
}

// CatalogSettlement is a settlement a catalog record is bound to. Its UUID is
// the one GetSettlement reads.
type CatalogSettlement struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Type string `json:"type"` // e.g. «село», «город»
}

// uuidRe matches a uuid in its canonical textual form.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// GetCatalog reads a record catalog by its key ("gwarmil") or uuid. It is public;
// a session adds nothing here but is sent when there is one. An unknown catalog
// is ErrNotFound.
func (c *Client) GetCatalog(ctx context.Context, keyOrUUID string) (*Catalog, error) {
	if err := checkCatalogKey(keyOrUUID); err != nil {
		return nil, err
	}
	var wire wireCatalog
	if err := c.getV1(ctx, "catalogs/"+url.PathEscape(keyOrUUID), &wire); err != nil {
		return nil, err
	}
	return wire.catalog(), nil
}

// GetCatalogPerson reads one catalog record by its catalog's key and its uuid —
// the CatalogKey and UUID of a "catalogPerson" search result. It is public; with
// a session the bearer is sent, which reveals the fields a catalog hides from
// anonymous readers. An unknown record, or one under another catalog, is
// ErrNotFound. A uuid that is not one is ErrInvalidRequest before any request:
// familio answers it with a 500, which the client would retry.
func (c *Client) GetCatalogPerson(ctx context.Context, catalogKey, uuid string) (*CatalogPerson, error) {
	if err := checkCatalogKey(catalogKey); err != nil {
		return nil, err
	}
	if !uuidRe.MatchString(uuid) {
		return nil, fmt.Errorf("%w: catalog record id %q is not a uuid", ErrInvalidRequest, uuid)
	}
	var wire wireExcerpt
	if err := c.getV1(ctx, "catalogs/"+url.PathEscape(catalogKey)+"/excerpts/"+uuid, &wire); err != nil {
		return nil, err
	}
	return wire.catalogPerson(), nil
}

// checkCatalogKey rejects a catalog key that cannot name a catalog route.
func checkCatalogKey(key string) error {
	if key == "" || strings.ContainsAny(key, "/?#") {
		return fmt.Errorf("%w: invalid catalog key %q", ErrInvalidRequest, key)
	}
	return nil
}

// getV1 GETs a public /api/v1 resource into out, sending the bearer only when
// the client has a session.
func (c *Client) getV1(ctx context.Context, path string, out any) error {
	req, err := c.newRequestAt(ctx, http.MethodGet, apiV1Path+path, nil, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/ld+json")
	if err := c.addOptionalBearer(ctx, req); err != nil {
		return err
	}
	return c.do(req, out)
}

// wireCatalog is GET /api/v1/catalogs/<key> as familio sends it.
type wireCatalog struct {
	UUID                 string `json:"uuid"`
	Key                  string `json:"key"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	IssueYearDescription string `json:"issueYearDescription"`
	Hidden               bool   `json:"hidden"`
	CatalogType          string `json:"catalogType"`
	RecordsCount         int    `json:"recordsCount"`
	CatalogsFields       []struct {
		Key                    string `json:"key"`
		Title                  string `json:"title"`
		Type                   string `json:"type"`
		ShowInCard             bool   `json:"showInCard"`
		ShowInTable            bool   `json:"showInTable"`
		HiddenFromUnauthorized bool   `json:"hiddenFromUnauthorized"`
	} `json:"catalogsFields"`
}

func (w wireCatalog) catalog() *Catalog {
	c := &Catalog{
		UUID:         w.UUID,
		Key:          w.Key,
		Name:         w.Name,
		Description:  w.Description,
		Years:        w.IssueYearDescription,
		Hidden:       w.Hidden,
		Type:         w.CatalogType,
		RecordsCount: w.RecordsCount,
	}
	for _, f := range w.CatalogsFields {
		c.Fields = append(c.Fields, CatalogField(f))
	}
	return c
}

// wireExcerpt is GET /api/v1/catalogs/<key>/excerpts/<uuid> as familio sends it.
type wireExcerpt struct {
	UUID         string         `json:"uuid"`
	RecordID     string         `json:"recordID"`
	ExcerptsText string         `json:"excerptsText"`
	CatalogKey   string         `json:"catalogKey"`
	Attributes   phpMap         `json:"attributes"`
	Record       map[string]any `json:"record"`
	BirthDate    *EventDate     `json:"birth_date"`
	DeathDate    *EventDate     `json:"death_date"`
	Persons      []struct {
		ID         string `json:"@id"` // "/persons/<uuid>"
		LastName   string `json:"lastName"`
		FirstName  string `json:"firstName"`
		MiddleName string `json:"middleName"`
		Sex        string `json:"sex"` // "m" | "f"
	} `json:"persons"`
	Settlements []struct {
		ID          string `json:"@id"` // "/settlements/<uuid>"
		PrimaryName struct {
			Name string `json:"name"`
		} `json:"primaryName"`
		Type struct {
			Name struct {
				Ru string `json:"ru"`
			} `json:"name"`
		} `json:"type"`
	} `json:"settlements"`
}

func (w wireExcerpt) catalogPerson() *CatalogPerson {
	p := &CatalogPerson{
		UUID:       w.UUID,
		CatalogKey: w.CatalogKey,
		RecordID:   w.RecordID,
		Text:       w.ExcerptsText,
		BirthDate:  w.BirthDate,
		DeathDate:  w.DeathDate,
		Attributes: map[string]any(w.Attributes),
		Record:     w.Record,
	}
	if p.Attributes == nil {
		p.Attributes = map[string]any{}
	}
	if geo, ok := p.Attributes["geography"].([]any); ok {
		for _, g := range geo {
			if s, ok := g.(string); ok && s != "" {
				p.Geography = append(p.Geography, s)
			}
		}
	}
	if place, ok := p.Attributes["birth_place"].(string); ok {
		p.BirthPlace = place
	}
	for _, lp := range w.Persons {
		p.LinkedPersons = append(p.LinkedPersons, CatalogLinkedPerson{
			UUID:       lastPathSegment(lp.ID),
			LastName:   lp.LastName,
			FirstName:  lp.FirstName,
			MiddleName: lp.MiddleName,
			Gender:     genderFromSex(lp.Sex),
		})
	}
	for _, s := range w.Settlements {
		p.Settlements = append(p.Settlements, CatalogSettlement{
			UUID: lastPathSegment(s.ID),
			Name: s.PrimaryName.Name,
			Type: s.Type.Name.Ru,
		})
	}
	return p
}

// phpMap decodes a JSON object into a map, and also accepts the empty array
// PHP's json_encode writes for an empty map.
type phpMap map[string]any

func (m *phpMap) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("[]")) || bytes.Equal(b, []byte("null")) {
		*m = phpMap{}
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*m = v
	return nil
}

// lastPathSegment returns the uuid at the end of a Hydra @id such as
// "/persons/<uuid>".
func lastPathSegment(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// genderFromSex maps familio's one-letter sex code to the Gender constants.
func genderFromSex(sex string) string {
	switch sex {
	case "m":
		return GenderMale
	case "f":
		return GenderFemale
	}
	return ""
}
