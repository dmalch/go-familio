package familio

import (
	"context"
	"net/http"
)

// Settlement is a place attached to an event — familio's «Место рождения /
// смерти / etc.». The write contract is a structured object, NOT a bare uuid: a
// bare string is rejected (HTTP 400). The minimal accepted body is
// `{"uuid": "<id>"}`; the server enriches the name and administrative requisites
// («реквизиты»), which come back on read but are not needed by the provider, so
// only the uuid is modelled (extra read fields are ignored). See the "Settlement
// / place on events" section of internal/familio/API.md.
type Settlement struct {
	UUID string `json:"uuid"`
}

// SettlementRef wraps a settlement uuid as an event place. An empty uuid yields
// nil — i.e. no place (which clears any existing place on an upsert).
func SettlementRef(uuid string) *Settlement {
	if uuid == "" {
		return nil
	}
	return &Settlement{UUID: uuid}
}

// SettlementUUID returns the event's place uuid, or "" when no place is set.
func (e *Event) SettlementUUID() string {
	if e.Settlement == nil {
		return ""
	}
	return e.Settlement.UUID
}

// SettlementDetail is the full settlement record from GET
// /api/v2/settlements/<uuid>: the canonical name, its administrative requisites
// («реквизиты»), classification and coordinates. Distinct from the Settlement
// write-ref above (which carries only the uuid). nearestSettlements is returned
// by the API but not surfaced by the provider.
type SettlementDetail struct {
	UUID             string        `json:"uuid"`
	PrimaryName      string        `json:"primaryName"`
	AdditionalNames  []string      `json:"additionalNames"`
	MainGeorequisite *Georequisite `json:"mainGeorequisite"`
	Type             string        `json:"type"`   // settlement kind, e.g. «село», «город».
	Status           string        `json:"status"` // e.g. «жилой» (inhabited).
	Coordinate       *Coordinate   `json:"coordinate"`
}

// Georequisite is one administrative «реквизит» of a settlement: the level-1
// (region/oblast) and level-2 (district/city) administrative units as of Year.
type Georequisite struct {
	Level1 string `json:"level1"`
	Level2 string `json:"level2"`
	Year   int    `json:"year"`
}

// Coordinate is a GeoJSON Point: Coordinates is [longitude, latitude].
type Coordinate struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

// LatLon returns (latitude, longitude, ok); ok is false when the coordinate is
// absent or malformed.
func (c *Coordinate) LatLon() (lat, lon float64, ok bool) {
	if c == nil || len(c.Coordinates) < 2 {
		return 0, 0, false
	}
	return c.Coordinates[1], c.Coordinates[0], true
}

// GetSettlement reads a settlement's full record (GET /api/v2/settlements/<uuid>,
// Bearer). Returns ErrNotFound for an unknown uuid.
func (c *Client) GetSettlement(ctx context.Context, uuid string) (*SettlementDetail, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "settlements/"+uuid, nil, nil)
	if err != nil {
		return nil, err
	}
	var out SettlementDetail
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
