package familio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// Record types a PersonSearch can narrow to (PersonSearch.Types). With none,
// the search covers all three.
const (
	PersonSearchMine       = "my_persons"          // the account's own persons; needs a session
	PersonSearchCatalog    = "catalog_persons"     // record-catalog («справочник») entries
	PersonSearchOtherUsers = "other_users_persons" // other accounts' persons visible to this one
)

// Orders a PersonSearch can sort by (PersonSearch.OrderBy). The default is
// SearchOrderScore, relevance to the name searched for.
const (
	SearchOrderScore      = "score"
	SearchOrderName       = "full_name"
	SearchOrderBirthPlace = "birth_settlement_name"
	SearchOrderUpdated    = "person_updated_at"
	SearchOrderBirth      = "min_birth_date"
	SearchOrderDeath      = "min_death_date"
)

// searchPageSize is the page size when PersonSearch.ItemsPerPage is unset — the
// web UI's default.
const searchPageSize = 20

// PersonSearch is a query for SearchPersons: familio's people search («Люди»)
// over the account's own persons, other accounts' visible persons and the
// record catalogs. Name matching is fuzzy («Мальчиков» also finds «Мальчеков»)
// unless the Exact flag is set. A search with no name criterion finds nothing.
type PersonSearch struct {
	LastName                string // fuzzy unless LastNameExact
	LastNameExact           bool
	FirstAndMiddleName      string // fuzzy unless FirstAndMiddleNameExact
	FirstAndMiddleNameExact bool
	Text                    string // free text over the whole name, e.g. "Мальчиков Иван"

	Types  []string   // PersonSearchMine, PersonSearchCatalog, PersonSearchOtherUsers; none = all
	Gender string     // GenderMale or GenderFemale; "" = either
	Born   *DateRange // birth date: a single date, or a Range (no Circa)
	Died   *DateRange // death date, likewise

	OrderBy      string // a SearchOrder* value; "" = SearchOrderScore
	Ascending    bool   // ascending instead of the default descending order
	Page         int    // 1-based page number; 0 means 1
	ItemsPerPage int    // page size; 0 means searchPageSize
}

// PersonSearchResult is one person a search found. Type says which kind it is:
// "regularPerson" (an account's tree person) carries the owner, gender, privacy
// and places, "catalogPerson" (a record-catalog entry) carries its catalog.
// Dates are familio's EventDate — nil when absent, with a nil First when
// unknown — and convert with RangeFromEventDate.
type PersonSearchResult struct {
	UUID                string     `json:"uuid"`
	Type                string     `json:"type"`
	DisplayName         string     `json:"displayName"`
	ShortDisplayName    string     `json:"shortDisplayName"`
	OriginalDisplayName string     `json:"originalDisplayName"`
	BirthDate           *EventDate `json:"birthDate"`
	DeathDate           *EventDate `json:"deathDate"`
	HasDeathEvent       bool       `json:"hasDeathEvent"`
	BirthSettlementText string     `json:"birthSettlementText"`
	UpdatedAt           string     `json:"updatedAt"`

	// Regular persons only.
	OwnerID             string            `json:"ownerId,omitempty"`
	Gender              string            `json:"gender,omitempty"`
	PrivacyType         string            `json:"privacyType,omitempty"`
	IsMine              bool              `json:"isMine,omitempty"`
	IsMe                bool              `json:"isMe,omitempty"`
	Tags                []int             `json:"tags,omitempty"`
	BirthPlace          *SettlementDetail `json:"birthPlace,omitempty"`
	DeathPlace          *SettlementDetail `json:"deathPlace,omitempty"`
	DeathSettlementText string            `json:"deathSettlementText,omitempty"`

	// Catalog persons only.
	CatalogKey  string `json:"catalogKey,omitempty"`
	CatalogName string `json:"catalogName,omitempty"`
}

// PersonSearchPage is one page of SearchPersons results.
type PersonSearchPage struct {
	Pager   Pager                `json:"pager"`
	Persons []PersonSearchResult `json:"data"`
}

// SearchPersons runs one page of a person search
// (GET /api/v3/persons — the only v3 endpoint this package uses). Page through
// by advancing PersonSearch.Page until Pager.TotalItems is reached.
//
// The search is public. With a session the bearer is sent, which adds the
// account's own private persons and makes PersonSearchMine usable; without one
// the call goes out anonymously, and asking for PersonSearchMine is
// ErrNotLoggedIn before any request — familio would answer that with a 409.
func (c *Client) SearchPersons(ctx context.Context, search PersonSearch) (*PersonSearchPage, error) {
	q, err := search.query()
	if err != nil {
		return nil, err
	}
	if slices.Contains(search.Types, PersonSearchMine) && !c.hasSessionCookie() {
		return nil, fmt.Errorf("%w: searching %s needs a session", ErrNotLoggedIn, PersonSearchMine)
	}

	req, err := c.newRequestAt(ctx, http.MethodGet, apiV3Path+"persons", q, nil)
	if err != nil {
		return nil, err
	}
	if err := c.addOptionalBearer(ctx, req); err != nil {
		return nil, err
	}

	var page PersonSearchPage
	if err := c.do(req, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// query encodes the search as the endpoint's query parameters. page,
// itemsPerPage, orderBy, orderDirection and mentions are all mandatory on the
// wire (familio answers 400 naming whichever is missing), so they are always
// set; mentions is sent false, as the web UI does.
func (s PersonSearch) query() (url.Values, error) {
	orderBy := s.OrderBy
	if orderBy == "" {
		orderBy = SearchOrderScore
	}
	direction := "desc"
	if s.Ascending {
		direction = "asc"
	}
	perPage := s.ItemsPerPage
	if perPage < 1 {
		perPage = searchPageSize
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(max(s.Page, 1)))
	q.Set("itemsPerPage", strconv.Itoa(perPage))
	q.Set("orderBy", orderBy)
	q.Set("orderDirection", direction)
	q.Set("mentions", "false")
	if s.LastName != "" {
		q.Set("lastName", s.LastName)
	}
	if s.LastNameExact {
		q.Set("lastNameExactMatch", "true")
	}
	if s.FirstAndMiddleName != "" {
		q.Set("firstAndMiddleName", s.FirstAndMiddleName)
	}
	if s.FirstAndMiddleNameExact {
		q.Set("firstAndMiddleNameExactMatch", "true")
	}
	if s.Text != "" {
		q.Set("name", s.Text)
	}
	for i, t := range s.Types {
		q.Set("types["+strconv.Itoa(i)+"]", t)
	}
	if s.Gender != "" {
		q.Set("gender", s.Gender)
	}
	if err := setSearchDate(q, "birthDate", s.Born); err != nil {
		return nil, err
	}
	if err := setSearchDate(q, "deathDate", s.Died); err != nil {
		return nil, err
	}
	return q, nil
}

// searchDateMinYear and searchDateMaxYear stand in for an open side of a date
// range — the bounds the web UI sends.
const (
	searchDateMinYear = 1
	searchDateMaxYear = 9999
)

// setSearchDate encodes r as familio's search date filter under prefix: the
// calendar, then [equal] with the parts that are set for a single date, or
// [from] and [till] for a range. familio rejects a range bound with any part
// missing, so a bound widens to cover its whole year or month — from the 1st of
// January, till the last day of December or of the month — and an open side of
// RangeAfter or RangeBefore becomes year 9999 or year 1. familio has no
// approximate dates in search, so Circa is an error.
func setSearchDate(q url.Values, prefix string, r *DateRange) error {
	if r == nil {
		return nil
	}
	if r.Circa || r.EndCirca {
		return fmt.Errorf("familio: %s: person search has no approximate (circa) dates", prefix)
	}
	if r.Year < searchDateMinYear || r.Year > searchDateMaxYear {
		return fmt.Errorf("familio: %s: year %d out of range", prefix, r.Year)
	}
	calendar := r.Calendar
	if calendar == "" {
		calendar = calendarGregorian
	}
	q.Set(prefix+"[calendar]", calendar)

	setPart := func(bound, part string, v int) {
		q.Set(prefix+"["+bound+"]["+part+"]", strconv.Itoa(v))
	}
	setBound := func(bound string, year, month, day int) {
		setPart(bound, "year", year)
		setPart(bound, "month", month)
		setPart(bound, "day", day)
	}

	switch r.Range {
	case "":
		setPart("equal", "year", r.Year)
		if r.Month != nil {
			setPart("equal", "month", *r.Month)
		}
		if r.Day != nil {
			setPart("equal", "day", *r.Day)
		}
	case RangeBetween:
		if r.EndYear == nil {
			return errors.New("familio: " + prefix + ": a between range needs an end year")
		}
		y, m, d := widenStart(r.Year, r.Month, r.Day)
		setBound("from", y, m, d)
		y, m, d = widenEnd(*r.EndYear, r.EndMonth, r.EndDay)
		setBound("till", y, m, d)
	case RangeAfter:
		y, m, d := widenStart(r.Year, r.Month, r.Day)
		setBound("from", y, m, d)
		setBound("till", searchDateMaxYear, 12, 31)
	case RangeBefore:
		setBound("from", searchDateMinYear, 1, 1)
		y, m, d := widenEnd(r.Year, r.Month, r.Day)
		setBound("till", y, m, d)
	default:
		return fmt.Errorf("familio: %s: unknown date range %q", prefix, r.Range)
	}
	return nil
}

// widenStart fills a range start's missing month and day with the earliest.
func widenStart(year int, month, day *int) (int, int, int) {
	m, d := 1, 1
	if month != nil {
		m = *month
	}
	if day != nil {
		d = *day
	}
	return year, m, d
}

// widenEnd fills a range end's missing month and day with the latest: December,
// and the month's real last day.
func widenEnd(year int, month, day *int) (int, int, int) {
	m := 12
	if month != nil {
		m = *month
	}
	if day != nil {
		return year, m, *day
	}
	// Day 0 of the next month is the last day of this one.
	return year, m, time.Date(year, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
