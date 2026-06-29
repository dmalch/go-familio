package familio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// Source types — the two "add source" flows familio offers. See API.md.
const (
	// SourceTypeCase is an archive document (дело) from the digitised
	// organization → fund → register → case catalog. Its CatalogKey is null.
	SourceTypeCase = "case"
	// SourceTypeCatalogPerson is a record from a people index
	// (persons?type=catalogPerson). Its CatalogKey names the source catalog.
	SourceTypeCatalogPerson = "catalog_person"
)

// Source is a person's source citation (the «Источники» sub-resource). It is an
// immutable reference to a catalogued entity (UUID + Type + CatalogKey) plus an
// editable Comment. Name/Requisites/Years/Catalog are server-derived and read
// back only (never sent). The referenced entity's UUID is the source's identity
// within the person, so a person cannot cite the same entity twice.
type Source struct {
	UUID       string         `json:"uuid"`
	Type       string         `json:"type"`
	Comment    string         `json:"comment"`
	Name       string         `json:"name,omitempty"`
	Requisites string         `json:"requisites,omitempty"`
	Years      string         `json:"years,omitempty"`
	Catalog    *SourceCatalog `json:"catalog,omitempty"`
	CreatedAt  string         `json:"createdAt,omitempty"`
	UpdatedAt  string         `json:"updatedAt,omitempty"`
}

// SourceCatalog is the server-derived catalog descriptor on a catalog_person
// source. The API returns it as an OBJECT ({key, hidden}); a `case` source omits
// it. UnmarshalJSON also tolerates a bare string or null for forward-compat.
type SourceCatalog struct {
	Key    string `json:"key"`
	Hidden bool   `json:"hidden"`
}

func (c *SourceCatalog) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' { // tolerate a bare string catalog
		return json.Unmarshal(b, &c.Key)
	}
	type alias SourceCatalog
	return json.Unmarshal(b, (*alias)(c))
}

// String renders the catalog for the computed `catalog` attribute: its key
// (the stable catalog id, e.g. "vss"), or "" when absent.
func (c *SourceCatalog) String() string {
	if c == nil {
		return ""
	}
	return c.Key
}

// SourceRef is the write body of a source create: the reference triple familio
// accepts. CatalogKey is sent explicitly (null for a `case`, the catalog id for
// a `catalog_person`) — it is write-only and never echoed on reads.
type SourceRef struct {
	UUID       string  `json:"uuid"`
	Type       string  `json:"type"`
	CatalogKey *string `json:"catalogKey"`
}

// sourceCommentPatch is the partial body of an in-place comment edit.
type sourceCommentPatch struct {
	Comment string `json:"comment"`
}

// FindSourceByID returns the source with the given (entity) uuid, or nil.
func FindSourceByID(sources []Source, uuid string) *Source {
	for i := range sources {
		if sources[i].UUID == uuid {
			return &sources[i]
		}
	}
	return nil
}

// GetPersonSources lists a person's source citations
// (GET /api/v2/persons/<uuid>/sources).
func (c *Client) GetPersonSources(ctx context.Context, personUUID string) ([]Source, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/"+personUUID+"/sources", nil, nil)
	if err != nil {
		return nil, err
	}
	var sources []Source
	if err := c.do(req, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// CreateSource attaches a source citation to a person
// (POST /api/v2/persons/<uuid>/sources). Returns the enriched source.
func (c *Client) CreateSource(ctx context.Context, personUUID string, ref SourceRef) (*Source, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodPost, "persons/"+personUUID+"/sources", nil, ref)
	if err != nil {
		return nil, err
	}
	var out Source
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSourceComment edits a source's comment in place
// (PATCH /api/v2/persons/<uuid>/sources/<sourceUuid>). Only the comment is
// mutable; the reference is fixed. Returns the updated source.
func (c *Client) UpdateSourceComment(ctx context.Context, personUUID, sourceUUID, comment string) (*Source, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodPatch, "persons/"+personUUID+"/sources/"+sourceUUID, nil, sourceCommentPatch{Comment: comment})
	if err != nil {
		return nil, err
	}
	var out Source
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSource removes a source citation from a person
// (DELETE /api/v2/persons/<uuid>/sources/<sourceUuid>) → 204.
func (c *Client) DeleteSource(ctx context.Context, personUUID, sourceUUID string) error {
	req, err := c.newAuthedRequest(ctx, http.MethodDelete, "persons/"+personUUID+"/sources/"+sourceUUID, nil, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}
