package familio

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
)

func TestSettlementRef(t *testing.T) {
	RegisterTestingT(t)
	Expect(SettlementRef("")).To(BeNil(), "empty uuid should yield no settlement")
	Expect(SettlementRef("40d1b180")).To(Equal(&Settlement{UUID: "40d1b180"}))
}

// TestSettlementWriteIsObjectWithUUID locks the confirmed contract: the wire
// settlement must be the structured {"uuid": …} object (a bare string is
// rejected by familio with 400), and only the uuid is sent.
func TestSettlementWriteIsObjectWithUUID(t *testing.T) {
	RegisterTestingT(t)
	b, err := json.Marshal(BirthEvent(nil, "self", nil, "40d1b180", ""))
	Expect(err).ToNot(HaveOccurred())

	var got struct {
		Settlement *struct {
			UUID string `json:"uuid"`
		} `json:"settlement"`
	}
	Expect(json.Unmarshal(b, &got)).To(Succeed())
	Expect(got.Settlement).ToNot(BeNil())
	Expect(got.Settlement.UUID).To(Equal("40d1b180"))

	// No place ⇒ settlement is explicit null (matches familio's create body).
	none, _ := json.Marshal(BirthEvent(nil, "self", nil, "", ""))
	var probe map[string]json.RawMessage
	Expect(json.Unmarshal(none, &probe)).To(Succeed())
	Expect(string(probe["settlement"])).To(Equal("null"))
}

// TestSettlementReadBack decodes the enriched object familio returns
// ({uuid,name,mainGeorequisite}) and confirms the provider recovers the uuid.
func TestSettlementReadBack(t *testing.T) {
	RegisterTestingT(t)
	const body = `{"uuid":null,"type":"birth","date":{"calendar":"gregorian","type":"equal","first":null,"second":null},
		"participants":[],"comment":"",
		"settlement":{"uuid":"40d1b180","name":"Нижняя Верея","mainGeorequisite":{"level1":"Нижегородская область","level2":"город Выкса","year":2019}}}`
	var ev Event
	Expect(json.Unmarshal([]byte(body), &ev)).To(Succeed())
	Expect(ev.SettlementUUID()).To(Equal("40d1b180"))

	// An event with no place reads back as "" (nil settlement).
	var empty Event
	Expect(json.Unmarshal([]byte(`{"type":"birth","settlement":null,"participants":[]}`), &empty)).To(Succeed())
	Expect(empty.SettlementUUID()).To(BeEmpty())
}

func TestEventBuildersCarryPlace(t *testing.T) {
	RegisterTestingT(t)
	birth := BirthEvent(nil, "self", nil, "s1", "")
	death := DeathEvent(nil, "self", "s1", "")
	baptism := BaptismEvent(nil, "self", "s1", "")
	Expect(birth.SettlementUUID()).To(Equal("s1"))
	Expect(death.SettlementUUID()).To(Equal("s1"))
	Expect(baptism.SettlementUUID()).To(Equal("s1"))
}

// TestSettlementDetailDecode decodes a real GET /api/v2/settlements/<uuid> body
// (Нижняя Верея), confirming the requisites, classification and GeoJSON
// coordinate map to the right fields (coordinates are [lon, lat]).
func TestSettlementDetailDecode(t *testing.T) {
	RegisterTestingT(t)
	const body = `{"uuid":"40d1b180-b739-4ecb-9ee5-ced6fefcd0d8","primaryName":"Нижняя Верея",
		"additionalNames":[],
		"mainGeorequisite":{"level1":"Нижегородская область","level2":"город Выкса","year":2019},
		"type":"село","status":"жилой",
		"coordinate":{"type":"Point","coordinates":[41.976302,55.2479772]},
		"nearestSettlements":[{"uuid":"227e549f","primaryName":"Верхняя Верея"}]}`
	var s SettlementDetail
	Expect(json.Unmarshal([]byte(body), &s)).To(Succeed())
	Expect(s.PrimaryName).To(Equal("Нижняя Верея"))
	Expect(s.AdditionalNames).To(BeEmpty())
	Expect(s.MainGeorequisite.Level1).To(Equal("Нижегородская область"))
	Expect(s.MainGeorequisite.Level2).To(Equal("город Выкса"))
	Expect(s.MainGeorequisite.Year).To(Equal(2019))
	Expect(s.Type).To(Equal("село"))
	Expect(s.Status).To(Equal("жилой"))

	lat, lon, ok := s.Coordinate.LatLon()
	Expect(ok).To(BeTrue())
	Expect(lat).To(BeNumerically("~", 55.2479772, 1e-6))
	Expect(lon).To(BeNumerically("~", 41.976302, 1e-6))
}

func TestCoordinateLatLonNilSafe(t *testing.T) {
	RegisterTestingT(t)
	var c *Coordinate
	_, _, ok := c.LatLon()
	Expect(ok).To(BeFalse())
	_, _, ok = (&Coordinate{Type: "Point", Coordinates: []float64{1}}).LatLon()
	Expect(ok).To(BeFalse(), "a malformed (<2) coordinate is not usable")
}
