package familio

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// matchesPageSize is the default per-page size for the matches list, matching
// both the UI's page size and the server's own default.
const matchesPageSize = 15

// Match statuses («Ожидающие» / «Подтверждённые» / «Отклонённые»). A match
// starts out undecided; confirming or rejecting it is reversible by moving it
// back to undecided.
const (
	MatchStatusUndecided = "undecided"
	MatchStatusConfirmed = "confirmed"
	MatchStatusRejected  = "rejected"
)

// Facet is one selectable value of a list filter, with the number of records
// carrying it.
type Facet struct {
	Item struct {
		Value        string `json:"value"`
		DisplayValue string `json:"displayValue"`
	} `json:"item"`
	Count int `json:"count"`
}

// MatchPerson is one side of a match: the person read shape (Person) plus the
// ownership fields the matches endpoint adds. The two variants are told apart
// by Type — a "regularPerson" is a tree person and carries OwnerID, privacy and
// places, while a "catalogPerson" is a record-catalog entry that carries
// CatalogKey/CatalogName and leaves the ownership fields zero.
type MatchPerson struct {
	Person
	OwnerID       string            `json:"ownerId,omitempty"`
	IsMine        bool              `json:"isMine,omitempty"`
	IsGrantedToMe bool              `json:"isGrantedToMe,omitempty"`
	PrivacyType   string            `json:"privacyType,omitempty"`
	Biography     string            `json:"biography,omitempty"`
	Updating      bool              `json:"updating,omitempty"`
	BirthPlace    *SettlementDetail `json:"birthPlace,omitempty"`
	DeathPlace    *SettlementDetail `json:"deathPlace,omitempty"`
}

// MatchScore is the per-field breakdown behind a match. The components are
// points awarded per compared field; they do not sum to Match.Score, which is
// a separate probability percentage.
type MatchScore struct {
	FirstName  int `json:"firstName"`
	LastName   int `json:"lastName"`
	MiddleName int `json:"middleName"`
	BirthDate  int `json:"birthDate"`
	DeathDate  int `json:"deathDate"`
	BirthPlace int `json:"birthPlace"`
}

// Match is one candidate duplicate: a person of the account's own («Добавил я»)
// paired with a person from another user's tree or from a record catalog. UUID
// is the match's own id — the handle the confirm/reject/undecide calls take,
// not a person uuid.
type Match struct {
	UUID          string      `json:"uuid"`
	Score         int         `json:"score"`
	Date          string      `json:"date"`
	Status        string      `json:"status"`
	OwnPerson     MatchPerson `json:"ownPerson"`
	ForeignPerson MatchPerson `json:"foreignPerson"`
	DetailedScore MatchScore  `json:"detailedScore"`
}

// MatchPage is one page-numbered page of matches. DataVersionMark stamps the
// match set the page was computed from; the UI echoes it back as the
// X-Base-Version optimistic-lock header on its bulk filter-wide writes.
type MatchPage struct {
	Data            []Match `json:"data"`
	Pager           Pager   `json:"pager"`
	DataVersionMark string  `json:"dataVersionMark"`
}

// MatchScrollCursor is the cursor envelope of the scroll endpoint: the last
// item of the page just returned, and whether more remain.
type MatchScrollCursor struct {
	LastItem string `json:"lastItem"`
	HasMore  bool   `json:"hasMore"`
}

// MatchScrollPage is one cursor-paged page of matches.
type MatchScrollPage struct {
	Data            []Match           `json:"data"`
	Pager           MatchScrollCursor `json:"pager"`
	DataVersionMark string            `json:"dataVersionMark"`
}

// MatchFilter narrows and pages the matches list. The zero value requests the
// first page of everything over the full 1–99 score window.
type MatchFilter struct {
	PersonIDs []string // the account's own person uuids
	UserIDs   []string // owners of the matched foreign persons
	Catalogs  []string // record-catalog keys, e.g. "vss" («Источник совпадения»)
	Dates     []string // match batch dates, "YYYY-MM-DD"
	Statuses  []string // MatchStatusUndecided / Confirmed / Rejected

	MinScore int // score window start; 0 means 1
	MaxScore int // score window end; 0 means 99

	Page         int // 1-based page number; 0 means 1
	ItemsPerPage int // page size; 0 means matchesPageSize
}

// matchFilterBody is the wire form of a MatchFilter. Every field is always
// encoded: the API answers 400 ("Неправильный формат фильтра <name>") when any
// of the seven keys is missing, and a Go nil slice would encode as null rather
// than [], so the slices are normalized to empty.
type matchFilterBody struct {
	Person        []string `json:"person"`
	User          []string `json:"user"`
	Catalog       []string `json:"catalog"`
	Date          []string `json:"date"`
	Status        []string `json:"status"`
	MinTotalScore int      `json:"minTotalScore"`
	MaxTotalScore int      `json:"maxTotalScore"`
}

// body encodes the filter as the endpoints' POST payload.
func (f MatchFilter) body() matchFilterBody {
	minScore := max(f.MinScore, 1)
	maxScore := f.MaxScore
	if maxScore < 1 {
		maxScore = 99
	}
	return matchFilterBody{
		Person:        orEmpty(f.PersonIDs),
		User:          orEmpty(f.UserIDs),
		Catalog:       orEmpty(f.Catalogs),
		Date:          orEmpty(f.Dates),
		Status:        orEmpty(f.Statuses),
		MinTotalScore: minScore,
		MaxTotalScore: maxScore,
	}
}

// orEmpty replaces a nil slice with an empty one so it encodes as [] not null.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// query encodes the page-numbered paging parameters. Both are optional on the
// wire (the server defaults to page 1, itemsPerPage 15) but are always sent so
// the page size is explicit.
func (f MatchFilter) query() url.Values {
	perPage := f.ItemsPerPage
	if perPage < 1 {
		perPage = matchesPageSize
	}
	q := url.Values{}
	q.Set("page", strconv.Itoa(max(f.Page, 1)))
	q.Set("itemsPerPage", strconv.Itoa(perPage))
	return q
}

// matchesPath builds an owner-addressed matches sub-path for the account that
// owns the active session.
func (c *Client) matchesPath(ctx context.Context, action string) (string, error) {
	owner, err := c.AccountUUID(ctx)
	if err != nil {
		return "", err
	}
	return "users/" + owner + "/matches" + action, nil
}

// ListMatches fetches one page of the account's matches («Совпадения») via
// POST /api/v2/users/<accountUuid>/matches/get-by-filters. Matches are
// regenerated monthly; the free tier only gets them for public persons and a
// subset of catalogs. Page through by advancing MatchFilter.Page until
// Pager.TotalItems is reached, or use ScrollMatches for a cursor sweep.
func (c *Client) ListMatches(ctx context.Context, filter MatchFilter) (*MatchPage, error) {
	path, err := c.matchesPath(ctx, "/get-by-filters")
	if err != nil {
		return nil, err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPost, path, filter.query(), filter.body())
	if err != nil {
		return nil, err
	}

	var page MatchPage
	if err := c.do(req, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// ScrollMatches fetches one cursor-paged page of matches via
// POST /api/v2/users/<accountUuid>/matches/get-by-filters-scroll. after is the
// previous page's Pager.LastItem ("" for the first page); keep going while
// Pager.HasMore. MatchFilter.Page is ignored — the cursor supersedes it.
func (c *Client) ScrollMatches(ctx context.Context, filter MatchFilter, after string) (*MatchScrollPage, error) {
	path, err := c.matchesPath(ctx, "/get-by-filters-scroll")
	if err != nil {
		return nil, err
	}

	perPage := filter.ItemsPerPage
	if perPage < 1 {
		perPage = matchesPageSize
	}
	q := url.Values{}
	q.Set("itemsPerPage", strconv.Itoa(perPage))
	if after != "" {
		q.Set("pageAfterItem", after)
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPost, path, q, filter.body())
	if err != nil {
		return nil, err
	}

	var page MatchScrollPage
	if err := c.do(req, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// MatchFilters is the facet vocabulary of the matches list: every batch date,
// own person, foreign tree owner, record catalog and status present in the
// account's matches, each with its match count.
type MatchFilters struct {
	Dates    []Facet `json:"dateFilter"`
	Persons  []Facet `json:"personFilter"`
	Users    []Facet `json:"userFilter"`
	Catalogs []Facet `json:"catalogFilter"`
	Statuses []Facet `json:"statusFilter"`
}

// GetMatchFilters fetches the matches facet vocabularies via
// POST /api/v2/users/<accountUuid>/matches/get-filters-data. Each facet's
// counts are computed with the other filters in filter applied (a facet does
// not narrow itself), so the zero filter yields the full vocabulary.
func (c *Client) GetMatchFilters(ctx context.Context, filter MatchFilter) (*MatchFilters, error) {
	path, err := c.matchesPath(ctx, "/get-filters-data")
	if err != nil {
		return nil, err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPost, path, nil, filter.body())
	if err != nil {
		return nil, err
	}

	var filters MatchFilters
	if err := c.do(req, &filters); err != nil {
		return nil, err
	}
	return &filters, nil
}

// ConfirmMatches marks matches as confirmed («Подтвердить») via
// POST /api/v2/users/<accountUuid>/matches/confirm-by-ids. ids are match uuids
// (Match.UUID), not person uuids. Reversible with UndecideMatches.
func (c *Client) ConfirmMatches(ctx context.Context, ids []string) error {
	return c.decideMatches(ctx, "/confirm-by-ids", ids)
}

// RejectMatches marks matches as rejected («Отклонить») via
// POST /api/v2/users/<accountUuid>/matches/reject-by-ids. ids are match uuids
// (Match.UUID), not person uuids. Reversible with UndecideMatches.
func (c *Client) RejectMatches(ctx context.Context, ids []string) error {
	return c.decideMatches(ctx, "/reject-by-ids", ids)
}

// UndecideMatches returns matches to the undecided state via
// POST /api/v2/users/<accountUuid>/matches/undecide-by-ids, undoing an earlier
// confirm or reject. ids are match uuids (Match.UUID).
func (c *Client) UndecideMatches(ctx context.Context, ids []string) error {
	return c.decideMatches(ctx, "/undecide-by-ids", ids)
}

// decideMatches posts the match uuids to one of the *-by-ids status endpoints.
// The body is a bare JSON array of uuids (not an object), and the response is
// discarded — any 2xx means the status was applied.
func (c *Client) decideMatches(ctx context.Context, action string, ids []string) error {
	if len(ids) == 0 {
		return errors.New("familio: no match uuids given")
	}

	path, err := c.matchesPath(ctx, action)
	if err != nil {
		return err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPost, path, nil, ids)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}
